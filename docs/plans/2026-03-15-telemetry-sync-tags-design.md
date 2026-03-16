# Issue 10: Telemetry, Sync & Tags — Design

## Scope

Telemetry computation (MPG, speed, buffer, fever chart, time-in-chain), refspec configuration for sync, tag resolution in the resolve chain, and wiring telemetry into milestone show.

## Telemetry

`internal/telemetry/telemetry.go` already has `Stats` and `Status` types. Add:

`Compute(issues []*issue.Issue, ms *milestone.Milestone) *Stats`
- **MPG**: total commits across all sessions of done issues / count of done issues
- **Speed**: done issues / weeks elapsed since first issue started
- **Buffer**: 50% of total issue count, refined by MPG variance
- **BufferBurned**: sum of (issue commits - MPG) for issues exceeding MPG, normalized
- **FeverStatus**: GREEN if burn% < progress%, YELLOW if roughly equal, RED if burn% > progress%+20
- **TimeInChain**: total active session time / total calendar time since first start

Wire `Compute` into `milestone show` to display real telemetry.

## Sync

Add refspec configuration to `EnsureInitialized`:
- Check if repo has a remote named "origin"
- If yes, add fetch/push refspecs for `refs/zhi/*` to the remote config
- Skip silently if no remote

## Tag Resolution

Add tag resolution to `resolve.ResolveRef` between HEAD and UUID prefix:
- Check `refs/zhi/_/tags/<input>` — if exists, read content to get target ref path
- This makes tags created by `--tag` resolvable in show/edit/etc.

## Files

- Modify: `internal/telemetry/telemetry.go` — add Compute function
- Modify: `internal/telemetry/telemetry_test.go` — Compute tests
- Modify: `internal/cli/milestone_show.go` — wire telemetry into show output
- Modify: `internal/cli/app.go` — add refspec config to EnsureInitialized
- Modify: `internal/resolve/resolve.go` — add tag resolution step
- Modify: `internal/resolve/resolve_test.go` — tag resolution tests

## Testing

**Telemetry** (4 tests): Compute with done issues (MPG/speed), empty issues (zero values), fever chart status, buffer calculation.

**Sync** (1 test): EnsureInitialized with remote adds refspecs.

**Tag resolution** (2 tests): resolve by tag name, resolve nonexistent tag falls through to UUID.
