# git zhi sync — design

ABOUTME: Design for `git zhi sync`, the porcelain that moves chain state
(refs/zhi/*) and the working branch between a repo and its remote.

Tracks: perigrin/git-zhi#5. Follow-on to the verify gate (#3 / #4).

## Problem

Chain state lives in `refs/zhi/_/*` and does not ride default `git push` /
`git fetch` / `git clone` — only an explicit `refs/zhi/*:refs/zhi/*` refspec
carries it. Today moving state is hand-rolled git plumbing.

The C&C round-trip needs two channels to both land: (a) `refs/zhi/*` for the
git-zhi metadata, and (b) a normal branch fast-forward for the working tree.
`verify` runs its acceptance-criterion commands against the **working tree**, so
if only (a) lands, the tower reports a false regression. A porcelain command
must keep these in step.

## Command surface

```
git zhi sync [--push | --pull] [--remote <name>]
```

- **Bare `git zhi sync`** → pull then push (full reconcile). Pull first so
  remote state is integrated before local state is published; never push over
  remote changes blindly.
- **`--push`** → push only. **`--pull`** → pull only. Mutually exclusive;
  supplying both is an error.
- **`--remote <name>`** → target remote, default `origin`.

### Pull (down)

1. Bring chain state down. **Do not** use a single wildcard fetch
   (`refs/zhi/*:refs/zhi/*`): git prunes any local ref whose source is absent on
   the remote (`[deleted] (none) -> refs/zhi/...`), destroying local-only chain
   state that has not been pushed — including the common multi-worker case where
   the remote has *some* but not *all* of the local zhi refs. Instead, enumerate
   the remote's zhi refs via read-only `git ls-remote` and fetch each as its own
   explicit `refs/zhi/X:refs/zhi/X` refspec. Per-ref refspecs never carry the
   delete-on-absent-source semantics, so local-only refs survive. Non-force (no
   leading `+`): a diverged shared ref is reported as a conflict, not silently
   overwritten.
2. Fast-forward the current branch from its upstream. If the branch has
   diverged (cannot fast-forward), **refuse** with a clear error rather than
   merging — guaranteeing a consistent working tree before any `verify`.

### Push (up)

1. `git push <remote> HEAD 'refs/zhi/*:refs/zhi/*'` — branch tip and chain
   state in one invocation.

## Implementation

Shell out to `git` via `exec.Command`. This sidesteps go-git's missing
push-refspec field (`RemoteConfig` exposes only `Fetch`) and reuses the user's
credentials, hooks, and config. Consistent with the existing `gitRevParse`
helper in `internal/cli/app.go`.

### Refspec note

`sync` deliberately fetches with the **non-force** `refs/zhi/*:refs/zhi/*`,
whereas `configureRemoteRefspecs` writes the `+force` form for plain
`git fetch origin`. This is intentional: plain fetch keeps its existing
behavior; `sync` is the careful porcelain that refuses to clobber local chain
edits. Documented so it is not mistaken for an inconsistency.

## Error handling and edge cases

- **No remote configured** → clear error (`remote "<name>" not found`).
- **No upstream / detached HEAD** → skip the branch fast-forward with a notice,
  but still fetch `refs/zhi/*` (the chain state is the point; the tower may be
  on a detached checkout).
- **Fetch fails** (network/auth) → error, abort. Nothing partially applied:
  fetch is first.
- **Branch diverged** (cannot fast-forward) → error: "current branch has
  diverged from <upstream>; resolve manually before sync." Working tree
  untouched. `refs/zhi/*` was already fetched (refs only, no tree change) — note
  it.
- **Push rejected** (non-fast-forward remote) → surface git's stderr verbatim;
  suggest `git zhi sync --pull` first.
- **`--push --pull` together** → error.

Ordering in bare `sync`: pull fully completes (including branch FF) before push
starts. If pull errors, push never runs.

Exec discipline: capture stdout+stderr; on failure include git's own message.
Each git invocation's command line is what the user could run by hand.

## Out of scope

Conflict / merge strategy for divergent zhi refs (two workers touching the same
issue). The non-force fetch surfaces divergence as an error; a `--force` flag is
a possible later addition.

## Code structure

New files:

- `internal/cli/sync.go` — `NewSyncCommand()` (cobra), wired into `root.go`.
  Flags `--push`, `--pull` (mutually exclusive), `--remote` (default `origin`).
- `internal/cli/sync_test.go` — tests over real on-disk repos.

Helpers (small, testable, each wraps `exec.Command`, returns wrapped errors with
git's stderr intact):

- `runFetch(remote)` → `git fetch <remote> refs/zhi/*:refs/zhi/*`
- `fastForwardBranch(remote)` → resolve upstream; FF or refuse on divergence;
  detached/no-upstream → notice + skip
- `runPush(remote)` → `git push <remote> HEAD refs/zhi/*:refs/zhi/*`

## Testing

Real git, no mocks (CLAUDE.md). Harness builds a bare remote plus two clones,
mirroring the spike in #5.

- **push**: issue + commit on clone A → `sync --push` → bare remote has the
  `refs/zhi/*` ref and the branch tip.
- **pull**: state pushed from A → clone B `sync --pull` → B sees the issue
  (`LoadAllIssues`) and B's branch fast-forwarded.
- **divergence**: B's branch diverges → `sync --pull` refuses, tree untouched.
- **bare sync**: round-trip A → B → A.
- **edge**: no remote → error; detached HEAD → fetches refs, notes skipped FF;
  `--push --pull` together → error.
