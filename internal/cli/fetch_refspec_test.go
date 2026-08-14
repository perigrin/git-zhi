// ABOUTME: Tests that git-zhi never configures a refs/zhi/* fetch refspec that
// ABOUTME: lands in the local namespace, where a pruning fetch deletes the chain.

package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/perigrin/git-zhi/internal/cli"
)

// zhiFetchSpecs returns the lines of .git/config mentioning a refs/zhi fetch
// refspec. It reads the file directly rather than going through OpenRepo,
// which repairs the config as a side effect and would mask what is on disk.
func zhiFetchSpecs(t *testing.T, dir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil {
		t.Fatalf("read .git/config: %v", err)
	}
	var found []string
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "fetch") && strings.Contains(trimmed, "refs/zhi/") {
			found = append(found, trimmed)
		}
	}
	return found
}

// TestEnsureInitialized_DoesNotConfigureZhiFetchRefspec verifies init leaves
// remote.<name>.fetch alone. A wildcard refspec whose destination is a local
// namespace makes refs/zhi/* a prune target, so a pruning fetch against a
// remote with no refs/zhi/* deletes every local one.
func TestEnsureInitialized_DoesNotConfigureZhiFetchRefspec(t *testing.T) {
	remote := newBareRemote(t)
	dir := cloneRepo(t, remote)
	addIssue(t, dir, "seed")

	if specs := zhiFetchSpecs(t, dir); len(specs) > 0 {
		t.Errorf("init configured a refs/zhi fetch refspec: %v\n"+
			"a pruning `git fetch` would then delete the chain", specs)
	}
}

// TestPruningFetchPreservesChain is the behavioural guarantee behind #19: a
// fetch run before the chain has ever been pushed must not destroy it.
//
// fetch.prune is set explicitly rather than inherited. Pruning is what
// actually triggers the deletion, and it comes from the user's environment —
// fetch.prune=true is a common global setting. Without setting it here the
// test passes even with the bug present, which is how the original version of
// this test managed to be green on CI and red only on a developer machine.
func TestPruningFetchPreservesChain(t *testing.T) {
	remote := newBareRemote(t)
	dir := cloneRepo(t, remote)

	runGit(t, dir, "config", "fetch.prune", "true")
	runGit(t, dir, "commit", "--allow-empty", "-m", "seed")
	runGit(t, dir, "push", "origin", "HEAD")
	addIssue(t, dir, "chain issue")

	before := countZhiRefs(t, dir)
	if before == 0 {
		t.Fatal("expected chain refs before fetch")
	}

	runGit(t, dir, "fetch", "origin")

	if after := countZhiRefs(t, dir); after != before {
		t.Errorf("pruning `git fetch` changed chain refs: %d -> %d (expected unchanged)", before, after)
	}
}

// TestOpenRepo_KeepsRemoteTrackingZhiRefspec verifies the repair is targeted.
// A refspec whose destination is under refs/remotes/ is the correct way to
// mirror another chain and must survive, along with the ordinary heads
// refspec — otherwise the repair is indiscriminate rather than a fix.
func TestOpenRepo_KeepsRemoteTrackingZhiRefspec(t *testing.T) {
	remote := newBareRemote(t)
	dir := cloneRepo(t, remote)
	addIssue(t, dir, "seed")

	safe := "+refs/zhi/*:refs/remotes/origin/zhi/*"
	runGit(t, dir, "config", "--add", "remote.origin.fetch", safe)
	runGit(t, dir, "config", "--add", "remote.origin.fetch", "+refs/zhi/*:refs/zhi/*")

	app, err := cli.OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	if !app.RemovedFetchRefspec {
		t.Error("expected RemovedFetchRefspec to record the repair")
	}

	raw, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil {
		t.Fatalf("read .git/config: %v", err)
	}
	cfg := string(raw)
	if strings.Contains(cfg, "+refs/zhi/*:refs/zhi/*") {
		t.Error("destructive refspec survived the repair")
	}
	if !strings.Contains(cfg, safe) {
		t.Errorf("repair removed the safe remote-tracking refspec %q:\n%s", safe, cfg)
	}
	if !strings.Contains(cfg, "refs/remotes/origin/*") {
		t.Errorf("repair removed the ordinary heads refspec:\n%s", cfg)
	}
}

// TestOpenRepo_RemovesDangerousZhiFetchRefspec verifies that a repo carrying
// the refspec written by earlier versions is healed on open, since the config
// is actively destructive and users cannot be expected to find it.
func TestOpenRepo_RemovesDangerousZhiFetchRefspec(t *testing.T) {
	remote := newBareRemote(t)
	dir := cloneRepo(t, remote)
	addIssue(t, dir, "seed")

	runGit(t, dir, "config", "--add", "remote.origin.fetch", "+refs/zhi/*:refs/zhi/*")
	if specs := zhiFetchSpecs(t, dir); len(specs) == 0 {
		t.Fatal("setup failed: refspec not present")
	}

	if _, err := cli.OpenRepo(dir); err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}

	if specs := zhiFetchSpecs(t, dir); len(specs) > 0 {
		t.Errorf("OpenRepo left the destructive refspec in place: %v", specs)
	}
}

// countZhiRefs returns the number of refs/zhi/* refs in the repo at dir.
func countZhiRefs(t *testing.T, dir string) int {
	t.Helper()
	out := runGit(t, dir, "for-each-ref", "--format=%(refname)", "refs/zhi/")
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}
