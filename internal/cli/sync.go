// ABOUTME: Implementation of git zhi sync: moves refs/zhi/* and the working
// ABOUTME: branch between a repo and its remote, shelling out to git.

package cli

import (
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// zhiRefspec carries chain state. Non-force: a diverged local zhi ref is
// reported as a conflict rather than silently overwritten. This is deliberately
// stricter than the +force fetch refspec configureRemoteRefspecs writes for
// plain `git fetch origin` — sync is the careful porcelain.
const zhiRefspec = "refs/zhi/*:refs/zhi/*"

// NewSyncCommand creates the top-level "sync" subcommand.
func NewSyncCommand() *cobra.Command {
	var doPush, doPull bool
	var remote string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Push and pull chain state (refs/zhi/*) and the working branch",
		Long: `Moves git-zhi chain state and the working branch between this repo and a
remote. Chain state lives in refs/zhi/* and does not ride default git
push/fetch; sync carries both channels together.

With no flags, sync pulls then pushes (a full reconcile, pulling first so
remote state is integrated before local state is published). --push or --pull
restrict to one direction.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if doPush && doPull {
				return fmt.Errorf("--push and --pull are mutually exclusive")
			}
			app := GetApp(cmd.Context())
			if app == nil {
				return fmt.Errorf("no git repository found")
			}
			dir, err := app.worktreeDir()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()

			switch {
			case doPush:
				return SyncPush(dir, remote, out)
			case doPull:
				return SyncPull(dir, remote, out)
			default:
				// Full reconcile: pull then push.
				if err := SyncPull(dir, remote, out); err != nil {
					return err
				}
				return SyncPush(dir, remote, out)
			}
		},
	}

	cmd.Flags().BoolVar(&doPush, "push", false, "push only (branch + refs/zhi/*)")
	cmd.Flags().BoolVar(&doPull, "pull", false, "pull only (fetch refs/zhi/* + fast-forward branch)")
	cmd.Flags().StringVar(&remote, "remote", "origin", "remote to sync with")
	return cmd
}

// SyncPush pushes the current branch tip and refs/zhi/* to remote in one step.
func SyncPush(dir, remote string, out io.Writer) error {
	if err := requireRemote(dir, remote); err != nil {
		return err
	}
	fmt.Fprintf(out, "Pushing branch and refs/zhi/* to %s...\n", remote)
	if _, err := runGitDir(dir, "push", remote, "HEAD", zhiRefspec); err != nil {
		return fmt.Errorf("push to %s failed (if rejected as non-fast-forward, run 'git zhi sync --pull' first): %w", remote, err)
	}
	return nil
}

// SyncPull fetches refs/zhi/* and fast-forwards the current branch from its
// upstream, ordered so the working tree is consistent before any verify runs.
// A diverged branch is refused rather than merged.
func SyncPull(dir, remote string, out io.Writer) error {
	if err := requireRemote(dir, remote); err != nil {
		return err
	}
	// Fetch each refs/zhi/* ref the remote advertises, as its own explicit
	// refspec. A single wildcard fetch (refs/zhi/*:refs/zhi/*) would PRUNE any
	// local ref whose source is absent on the remote — git reports
	// "[deleted] (none) -> refs/zhi/..." — destroying local-only chain state
	// that has not been pushed yet. Explicit per-ref refspecs never carry that
	// delete-on-absent-source semantics, so local-only refs survive. ls-remote
	// is read-only and safe.
	remoteRefs, err := remoteZhiRefs(dir, remote)
	if err != nil {
		return err
	}
	if len(remoteRefs) == 0 {
		fmt.Fprintf(out, "Remote %s has no refs/zhi/* yet; skipping chain fetch.\n", remote)
	} else {
		fmt.Fprintf(out, "Fetching %d refs/zhi/* ref(s) from %s...\n", len(remoteRefs), remote)
		args := []string{"fetch", remote}
		for _, r := range remoteRefs {
			// Non-force (no leading +): a diverged shared ref is rejected, not
			// clobbered.
			args = append(args, r+":"+r)
		}
		if _, err := runGitDir(dir, args...); err != nil {
			return fmt.Errorf("fetch refs/zhi/* from %s failed: %w", remote, err)
		}
	}
	return fastForwardBranch(dir, remote, out)
}

// fastForwardBranch fast-forwards the current branch to its upstream after a
// fetch. Detached HEAD or no upstream is a notice, not an error (the chain refs
// already arrived). A diverged branch is refused.
func fastForwardBranch(dir, remote string, out io.Writer) error {
	branch, err := currentBranch(dir)
	if err != nil || branch == "" {
		fmt.Fprintln(out, "notice: detached HEAD; fetched refs/zhi/* but skipped branch fast-forward")
		return nil
	}

	// A local branch with no counterpart on the remote has no upstream to
	// fast-forward from. This is a notice, not an error — the chain refs already
	// arrived. Checking ls-remote first avoids a hard `git fetch` failure.
	onRemote, err := remoteHasBranch(dir, remote, branch)
	if err != nil {
		return err
	}
	if !onRemote {
		fmt.Fprintf(out, "notice: branch %s not on %s; fetched refs/zhi/* but skipped branch fast-forward\n", branch, remote)
		return nil
	}

	// Fetch the branch itself so its remote-tracking ref is current. The "--"
	// guards against a branch name that begins with "-" being read as an option.
	if _, err := runGitDir(dir, "fetch", remote, "--", branch); err != nil {
		return fmt.Errorf("fetch branch %s from %s failed: %w", branch, remote, err)
	}

	upstream := remote + "/" + branch
	localSHA, err := revParse(dir, "HEAD")
	if err != nil {
		return err
	}
	remoteSHA, err := revParse(dir, "refs/remotes/"+upstream)
	if err != nil {
		// The branch is on the remote but its remote-tracking ref did not
		// resolve (e.g. ref absent locally). Treat as no upstream rather than
		// masking an unexpected error silently.
		fmt.Fprintf(out, "notice: no upstream %s; fetched refs/zhi/* but skipped branch fast-forward\n", upstream)
		return nil
	}

	if localSHA == remoteSHA {
		fmt.Fprintf(out, "Branch %s already up to date with %s.\n", branch, upstream)
		return nil
	}

	switch {
	case isAncestor(dir, localSHA, remoteSHA):
		// Remote is ahead: fast-forward the local branch up to it.
		if _, err := runGitDir(dir, "merge", "--ff-only", upstream); err != nil {
			return fmt.Errorf("fast-forward %s to %s failed: %w", branch, upstream, err)
		}
		fmt.Fprintf(out, "Fast-forwarded %s to %s.\n", branch, upstream)
		return nil
	case isAncestor(dir, remoteSHA, localSHA):
		// Local is ahead of remote: nothing to fast-forward; a subsequent push
		// publishes the local commits.
		fmt.Fprintf(out, "Branch %s is ahead of %s; nothing to fast-forward.\n", branch, upstream)
		return nil
	default:
		// Neither is an ancestor of the other: genuinely diverged.
		return fmt.Errorf("current branch %s has diverged from %s; resolve manually (rebase/merge) before sync", branch, upstream)
	}
}

// remoteZhiRefs returns the refs/zhi/* ref names the remote advertises.
// ls-remote is read-only, so this never mutates local refs. Output lines are
// "<sha>\t<refname>"; only the ref names are returned.
func remoteZhiRefs(dir, remote string) ([]string, error) {
	out, err := runGitDir(dir, "ls-remote", remote, "refs/zhi/*")
	if err != nil {
		return nil, fmt.Errorf("query refs/zhi/* on %s: %w", remote, err)
	}
	var refs []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		_, ref, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		ref = strings.TrimSpace(ref)
		if strings.HasPrefix(ref, "refs/zhi/") {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

// remoteHasBranch reports whether the remote advertises refs/heads/<branch>.
// The full refs/heads/ form is used so matching is exact: a bare name matches
// any trailing path component (e.g. "bar" would match "refs/heads/foo/bar"),
// which would bypass the no-upstream skip and cause a hard fetch failure.
// ls-remote is read-only.
func remoteHasBranch(dir, remote, branch string) (bool, error) {
	out, err := runGitDir(dir, "ls-remote", "--heads", remote, "refs/heads/"+branch)
	if err != nil {
		return false, fmt.Errorf("query branch %s on %s: %w", branch, remote, err)
	}
	return strings.TrimSpace(out) != "", nil
}

// requireRemote returns an error if the named remote is not configured.
func requireRemote(dir, remote string) error {
	out, err := runGitDir(dir, "remote")
	if err != nil {
		return fmt.Errorf("list remotes: %w", err)
	}
	if slices.Contains(strings.Fields(out), remote) {
		return nil
	}
	return fmt.Errorf("remote %q not found", remote)
}

// currentBranch returns the current branch name, or "" when HEAD is detached.
func currentBranch(dir string) (string, error) {
	out, err := runGitDir(dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		// Non-zero exit means detached HEAD; not a hard error.
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

// revParse returns the SHA the rev resolves to.
func revParse(dir, rev string) (string, error) {
	out, err := runGitDir(dir, "rev-parse", rev)
	if err != nil {
		return "", fmt.Errorf("rev-parse %s: %w", rev, err)
	}
	return strings.TrimSpace(out), nil
}

// isAncestor reports whether maybeAncestor is an ancestor of other (i.e. a
// fast-forward from maybeAncestor to other is possible).
func isAncestor(dir, maybeAncestor, other string) bool {
	cmd := exec.Command("git", "-C", dir, "merge-base", "--is-ancestor", maybeAncestor, other)
	return cmd.Run() == nil
}

// runGitDir runs `git -C dir <args...>` and returns combined output. On failure
// the error includes git's own output so the cause is not swallowed.
func runGitDir(dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
