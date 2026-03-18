# Busybox-Style Single Binary Design

One binary serves all eight git-zhi commands. The binary checks `os.Args[0]` at startup and dispatches to the matching command constructor. Symlinks provide the names that git's plugin discovery expects.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Dispatch mechanism | `filepath.Base(os.Args[0])` | Busybox pattern; no Cobra restructuring needed |
| Symlink location | Same directory as the binary | Already on `$PATH`; no extra configuration |
| Setup command | `git zhi setup` | Idempotent symlink creation; runs after install or update |
| Windows support | `os.Link` (hardlink), fallback to `.cmd` wrappers | Symlinks require elevated privileges on Windows; hardlinks do not |
| Separate entry points | Delete `cmd/git-zhi-*/` directories | One entry point eliminates version drift |

## Dispatch Map

```go
var dispatch = map[string]func() *cobra.Command{
    "git-zhi":           cli.NewRootCommand,
    "git-zhi-historian": historian.NewHistorianCommand,
    "git-zhi-jira":      jira.NewJiraCommand,
    "git-zhi-project":   project.NewProjectCommand,
    "git-zhi-mermaid":   mermaid.NewMermaidCommand,
    "git-zhi-verify":    verify.NewVerifyCommand,
    "git-zhi-sanbao":    sanbao.NewSanbaoCommand,
    "git-zhi-docs":      docs.NewDocsCommand,
}
```

When the binary name matches no key, fall back to `git-zhi` (the core command tree).

## App Context Wiring

Plugins that access git refs need a `Store` and `Repo` injected via context. Plugins that read stdin or open repos themselves do not.

| Binary name | Needs repo | Reason |
|-------------|-----------|--------|
| `git-zhi` | yes | Core commands read/write refs |
| `git-zhi-historian` | yes | Writes issues to refs |
| `git-zhi-jira` | yes | Reads issues, writes snapshots |
| `git-zhi-verify` | yes | Reads issues and acceptance criteria |
| `git-zhi-sanbao` | yes | Reads issues for metrics |
| `git-zhi-docs` | yes | Reads repo for doc health |
| `git-zhi-mermaid` | no | Pure formatter: JSON stdin, text stdout |
| `git-zhi-project` | no | Opens repos itself from project YAML |

The dispatch function opens the repo and injects context only when the command requires it. Commands that manage their own repo access skip this step.

## Setup Subcommand

`git zhi setup` creates symlinks (or hardlinks on Windows) for every companion name in the dispatch map. It resolves the binary's own path via `os.Executable()` and creates links in the same directory.

```
$ git zhi setup
Created symlink: /usr/local/bin/git-zhi-historian -> git-zhi
Created symlink: /usr/local/bin/git-zhi-jira -> git-zhi
Created symlink: /usr/local/bin/git-zhi-project -> git-zhi
Created symlink: /usr/local/bin/git-zhi-mermaid -> git-zhi
Created symlink: /usr/local/bin/git-zhi-verify -> git-zhi
Created symlink: /usr/local/bin/git-zhi-sanbao -> git-zhi
Created symlink: /usr/local/bin/git-zhi-docs -> git-zhi
```

Idempotent: existing symlinks pointing to the binary are left alone. Stale symlinks (pointing elsewhere) are replaced with a warning.

## Windows Support

Windows does not support symlinks without elevated privileges. The setup command tries `os.Link` (hardlink) first, which works on NTFS without elevation. If hardlinks fail (FAT32, network drives), it creates `.cmd` wrapper scripts:

```cmd
@echo off
"%~dp0git-zhi.exe" %*
```

The `.cmd` wrappers invoke the main binary with the original arguments. Git discovers `git-zhi-historian.cmd` the same way it discovers `git-zhi-historian.exe`.

## Build and Release

One build target replaces eight:

```makefile
build:
	go build -o git-zhi ./cmd/git-zhi/
```

GitHub Actions produces one binary per platform (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64). The release artifact is a single file plus checksums.

The existing updater (`internal/updater/`) requires no changes. It downloads and replaces one binary. Symlinks continue pointing to it.

## Crochet Integration

Crochet's install process:

1. Download `git-zhi` binary for the current platform from GitHub releases
2. Place in a directory on `$PATH` (or `~/.local/bin/`)
3. Run `git zhi setup` to create companion symlinks

One HTTP fetch, one shell command.

## What Changes

**Created:**
- Dispatch logic in `cmd/git-zhi/main.go`
- `git zhi setup` subcommand (new file in `internal/cli/`)

**Deleted:**
- `cmd/git-zhi-historian/main.go`
- `cmd/git-zhi-jira/main.go`
- `cmd/git-zhi-mermaid/main.go`
- `cmd/git-zhi-project/main.go`
- `cmd/git-zhi-verify/main.go`
- `cmd/git-zhi-sanbao/main.go`
- `cmd/git-zhi-docs/main.go`

**Modified:**
- `cmd/git-zhi/main.go` — absorbs dispatch logic
- `Makefile` — one build target
- `CLAUDE.md`, `CONTRIBUTORS.md` — update build instructions
- GitHub Actions workflow — one binary per platform

**Unchanged:**
- All `internal/` packages (command constructors, domain logic, tests)
- The updater (`internal/updater/`)
- The `NewXCommand()` functions in each plugin package

## Implementation Order

1. Write dispatch logic and `setup` subcommand with tests
2. Update `cmd/git-zhi/main.go` to use dispatch
3. Delete separate entry points (`cmd/git-zhi-*/`)
4. Update build instructions and Makefile
5. Verify all tests pass
6. Update documentation
