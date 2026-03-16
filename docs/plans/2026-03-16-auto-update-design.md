# Auto-Update System — Design

## Scope

Self-update system for the `git-zhi` binary, modeled after the PVM updater. Checks GitHub releases, downloads platform-appropriate assets, performs atomic binary replacement with backup/rollback, and supports background update checking with configurable intervals.

## Source

Ported from `~/dev/pvm/internal/updater/`, `~/dev/pvm/internal/version/`, and `~/dev/pvm/internal/download/`. Shell integration module dropped (not relevant — git-zhi is a standalone binary, not a shell environment manager).

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Package layout | Mirror PVM: `internal/updater/`, expand `internal/version/`, add `internal/download/` | Keeps cross-pollination easy; `internal/version/` already exists with build-time vars |
| Dependencies | stdlib only (`net/http`, `crypto/sha256`, `os`, `runtime`) | PVM's updater is pure stdlib; no new go.mod entries needed |
| Shell integration | Drop | git-zhi is a single binary on PATH; no shell config to update |
| Repository default | `perigrin/git-zhi` | Hardcoded default, overridable via config |
| Asset naming | `git-zhi-{os}-{arch}[.exe]` | Matches existing release workflow output |
| Backup location | `~/.cache/git-zhi/backups/` (XDG_CACHE_HOME) | Standard XDG convention, fallback to temp dir |
| Auto-update config | `~/.config/git-zhi/auto-update.json` (XDG_CONFIG_HOME) | Standard XDG convention |
| Update channels | stable, beta, alpha | Simplified from PVM's 5 channels; nightly/developer not needed |

## Package Structure

### `internal/version/` (expand existing)

The existing `version.go` (build-time vars, `GetVersion()`, `GetBuildInfo()`) stays as-is. Add:

- **`semver.go`** — Semantic version parsing and comparison. `SemanticVersion` struct with `Major`, `Minor`, `Patch`, `Prerelease`, `Build`, `Original` fields. Methods: `Parse()`, `Compare()`, `IsNewer()`, `IsOlder()`, `IsEqual()`. Prerelease ordering: numeric components compared numerically, then lexically; non-prerelease > prerelease.

- **`github.go`** — GitHub API client for fetching releases. `GitHubClient` struct with optional auth token (reads `GITHUB_TOKEN` from env). Methods: `GetLatestRelease()`, `GetReleases()`, `GetReleaseByTag()`. Rate limit handling with exponential backoff (capped at 5 minutes). Reduced backoff in test environments (detects `.test` suffix in `os.Args[0]`). Removes PVM's `isPVMRelease()` filter — git-zhi has only one release type.

- **`platform.go`** — Platform detection and asset matching. `Platform` struct with `OS`, `Architecture`, `Extension` fields. `DetectPlatform()` uses `runtime.GOOS`/`runtime.GOARCH`. `SelectBestAsset()` matches against release assets using pattern `git-zhi-{os}-{arch}`. Supports: linux-amd64, linux-arm64, darwin-arm64, windows-amd64.

- **`types.go`** — Shared types: `UpdateInfo` (current/latest version + release details), `VersionCheckResult`, `CheckOptions`, `GitHubClientInterface` (for dependency injection). No default repository — caller must provide.

### `internal/download/`

- **`downloader.go`** — HTTPS download with progress, retry, and checksum validation. `Downloader` struct with `DownloadOptions` (URL, destination, expected SHA256, progress callback, max retries, retry delay). 32KB streaming buffer. Progress callbacks at 100ms intervals. Resume support via Range headers. SHA256 validation on completion. Cache check (skip download if file exists with matching checksum). 30-minute timeout. User-Agent: `git-zhi-updater/1.0`.

### `internal/updater/`

- **`updater.go`** — Main orchestrator. `Updater` struct composes `GitHubClient`, `Downloader`, `BinaryReplacer`, `RollbackManager`, `RecoveryManager`. `PerformUpdate(opts)` coordinates the full flow through 8 stages (checking version, detecting platform, downloading, validating, creating backup, replacing, validating update, done — shell integration stage removed from PVM's 10). `UpdateOptions`: `TargetVersion`, `IncludePrerelease`, `Repository`, `Force`, `DryRun`, `Backup` (default true), `AutoRollback` (default true), `ProgressCallback`. Default repository: `perigrin/git-zhi`.

- **`backup.go`** — Timestamped binary backups with metadata. `BackupManager` creates backups at `~/.cache/git-zhi/backups/` with pattern `git-zhi-binary.backup.20060102-150405`. JSON metadata sidecar (`.meta` extension) with SHA256, original path, timestamp, version. Methods: `CreateBackup()`, `ListBackups()`, `GetLatestBackup()`, `RestoreBackup()`, `ValidateBackup()`, `CleanupOldBackups()`. Fallback to `$TMPDIR/git-zhi-backups/` if XDG cache unavailable.

- **`replacer.go`** — Atomic binary replacement with safety checks. Writes new binary to temp file in same directory, then atomic rename. Validates: file size (1MB–100MB), executable format. Preserves file permissions and ownership (Unix). Detects running processes (warning on Unix, blocking on Windows). `DetectInstallationMethod()` returns install method (binary, homebrew, apt, snap, etc.) with appropriate update instructions — refuses self-update for package manager installs unless `IgnoreInstallMethod` is set.

- **`recovery.go`** — Multi-strategy failure recovery. 8 scenarios: corrupted binary, partial update, permission denied, filesystem full, network failure, checksum mismatch, incompatible binary, unknown. `DiagnoseFailure()` classifies errors by keyword analysis and binary state checks. Each scenario has priority-ordered recovery strategies (rollback to latest, rollback to stable, fix permissions, clean temp, re-download, etc.). Max 3 recovery attempts per scenario.

- **`rollback.go`** — Manual and automatic rollback. `RollbackManager` wraps `BackupManager` and `BinaryReplacer`. `PerformRollback()` restores from specified or latest backup. `AutoRollback()` triggered on update failure when `AutoRollback` option is set. `ValidatePostRollback()` verifies restored binary is executable.

- **`auto_update.go`** — Background update checking. `AutoUpdateManager` with `AutoUpdateConfig` persisted as JSON. Config fields: `Enabled`, `CheckInterval` (default 24h), `Channel` (stable/beta/alpha), `LastCheckTime`, `NotificationDelay` (default 4h), `Repository`, `QuietMode`, `AutoInstall`. `CheckForUpdates()` respects check interval (no-op if checked recently). Returns `UpdateNotification` with current/latest version, release notes, download size, security flag. No auto-install scheduling (simplified from PVM's day-of-week/time window).

### `internal/cli/update.go`

The `git zhi update` command. Subcommands:

- **`git zhi update`** (default) — Check for and install updates interactively. Shows current vs. latest version, changelog, prompts for confirmation (or `--yes` to skip). Progress bar during download. Reports backup location on success.

- **`git zhi update check`** — Check only, don't install. Exits 0 if up-to-date, 1 if update available. JSON output with `--format json`.

- **`git zhi update rollback`** — Restore from most recent backup. `--list` to show available backups.

- **`git zhi update config`** — View/modify auto-update settings. `git zhi update config set auto-check true`, `git zhi update config set channel beta`, etc.

## Porting Approach

Files are ported from PVM with these systematic changes:

1. **String replacements**: `"pvm"` → `"git-zhi"`, `"perigrin/pvm"` → `"perigrin/git-zhi"`, `"PVM"` → `"git-zhi"`, `pvm-` → `git-zhi-` in asset patterns and temp dirs.
2. **Import paths**: `github.com/perigrin/pvm/internal/...` → `github.com/perigrin/git-zhi/internal/...`
3. **ABOUTME comments**: Add 2-line ABOUTME header to every file.
4. **Remove PVM-specific logic**: `isPVMRelease()` filter, shell integration stage, PVM-specific channel types (nightly, developer).
5. **Parameterize**: Repository, binary name, backup dir, user-agent — all configurable rather than hardcoded to PVM values.

## Integration with Existing Code

- `internal/version/version.go` unchanged — new files added alongside it.
- `internal/cli/root.go` gains `NewUpdateCommand()` registration. `"update"` added to `builtinNames` map.
- The `update` command does NOT require a git repo — `PersistentPreRunE` must be updated to skip repo opening for `update` and its subcommands (same pattern as `version`).

## Testing

Tests use the same patterns as the rest of git-zhi: real filesystems via `t.TempDir()`, no mocks.

**version/ tests:**
- `semver_test.go` — Parse valid/invalid versions, comparison ordering, prerelease vs. release, edge cases (build metadata, multi-part prerelease).
- `github_test.go` — HTTP test server serving canned release JSON. Rate limit header handling. Token auth header presence. Error responses (404, 500, rate limited).
- `platform_test.go` — DetectPlatform matches runtime. Asset selection from mixed asset lists. Unsupported platform rejection.

**download/ tests:**
- `downloader_test.go` — HTTP test server serving binary data. Checksum validation pass/fail. Resume from partial download. Retry on transient error. Progress callback invocation. Timeout handling.

**updater/ tests:**
- `backup_test.go` — Create/list/restore/validate/cleanup cycle. Metadata round-trip. Fallback directory selection.
- `replacer_test.go` — Atomic replacement on temp binary. Permission preservation. Size validation rejection. Installation method detection (test with various path patterns).
- `rollback_test.go` — Rollback from backup. Auto-rollback after simulated failure. Post-rollback validation.
- `recovery_test.go` — Diagnosis from error messages. Strategy selection per scenario. Max retry enforcement.
- `auto_update_test.go` — Config persistence round-trip. Check interval respected. Notification delay respected. Channel filtering.
- `updater_test.go` — Full update flow with HTTP test server (serve release JSON + binary asset). Dry run produces no side effects. Force flag skips version check. Backup created before replacement.

**cli/ tests:**
- `update_test.go` — `update check` output (human and JSON). Rollback list output. Config get/set round-trip.
