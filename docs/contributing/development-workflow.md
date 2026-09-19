---
stability: 2
covers:
  - Makefile
  - .github/workflows
  - install.sh
---

## Setup

Go 1.24+ and git. Clone, then:

```bash
make build      # stamps version, commit and build time
make test       # go test ./... -count=1
make vet
```

Build with `make`, not `go build` directly. The Makefile is the single
definition of `VERSION`, `COMMIT` and `LDFLAGS`, and the release workflow
calls it, so a local binary and a released one report their version the same
way. A binary built with bare `go build` says `unknown (built without make)`
rather than inventing a number.

## Branching

Feature branches are cut from `pu`, the default branch, and merge back to
`pu`. Never push directly to `pu`, `main` or `master`; never force-push a
shared branch.

To bring in newer work, rebase and push with `--force-with-lease`, and open
pull requests from a freshly rebased branch rather than merging the base
branch into the feature.

Rebasing rewrites every SHA on the branch. git-zhi records a session's start
SHA when an issue moves to `in-progress`, so a rebase mid-issue orphans that
reference; the tooling tolerates it and closes the window with zero commits
rather than refusing, but the count for that session is lost.

## Review

Every change goes through a pull request, and the test suite must be green.
Never `--no-verify`.

Changes arrive as a failing test first. Where the logic is subtle, confirm the
test can fail: revert the fix, watch it go red, restore it. A test that passes
before and after the change is documentation, not verification — which is
worth having, as long as it is labelled honestly.

## Release

Tag an annotated `vX.Y.Z` on `pu` and push the tag. That fires
`.github/workflows/release.yml`, which cross-compiles four platforms through
`make build` and publishes a GitHub Release.

Tags are the public surface: the tag message becomes the release notes, and
pushing one builds and publishes binaries. Let CI go green on `pu` before
tagging.

Version strings drop the leading `v` — the tag is `v0.5.2` and the binary
reports `0.5.2`, which is what every release has emitted since v0.4.0.
