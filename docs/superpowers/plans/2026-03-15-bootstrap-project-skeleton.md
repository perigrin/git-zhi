# Bootstrap Project Skeleton — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create the git-chain project skeleton so it compiles, runs `git-chain --help`, and has all internal packages stubbed with passing tests.

**Architecture:** Single Go binary at `cmd/git-chain/main.go`. Cobra root command with subcommand groups (`issue`, `milestone`, top-level `list`/`config`/`next`). Internal packages under `internal/` own domain logic. Each package starts as a stub with its ABOUTME comment and a placeholder test.

**Tech Stack:** Go 1.22+, Cobra (github.com/spf13/cobra), go-git (github.com/go-git/go-git/v5), gofrs/uuid, goccy/go-yaml

---

## File Structure

```
git-chain/
  cmd/git-chain/main.go            # Entry point: creates root command, calls Execute
  internal/
    cli/
      root.go                      # NewRootCommand(), Execute(), --format flag
      root_test.go                 # Root command prints help, subcommands registered
      app.go                       # App struct (Store + Repo), context key, accessor
      issue.go                     # NewIssueCommand() with add/list/show/edit stubs
      milestone.go                 # NewMilestoneCommand() with add/list/show/edit stubs
      chain.go                     # Top-level command stubs: list, config, next
    storage/
      store.go                     # Store struct, constructor
      store_test.go                # Constructor test (bare in-memory repo)
    issue/
      issue.go                     # Issue struct, State type, constants
      issue_test.go                # Type construction test
    milestone/
      milestone.go                 # Milestone struct
      milestone_test.go            # Type construction test
    graph/
      graph.go                     # Graph struct, constructor
      graph_test.go                # Empty graph test
    telemetry/
      telemetry.go                 # Stats struct, Status type
      telemetry_test.go            # Type construction test
    config/
      config.go                    # Config struct
      config_test.go               # Default config test
    resolve/
      resolve.go                   # Resolve function signature
      resolve_test.go              # Placeholder test
  go.mod
  go.sum
```

---

## Task 1: Initialize Go module, dependencies, and .gitignore

**Files:**
- Create: `go.mod`
- Modify: `.gitignore`

- [ ] **Step 1: Initialize Go module**

Run from the worktree root (`/home/perigrin/dev/git-chain/.worktrees/bootstrap`):

```bash
go mod init github.com/perigrin/git-chain
```

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/spf13/cobra@v1.9.1
go get github.com/go-git/go-git/v5@v5.16.0
go get github.com/gofrs/uuid/v5@v5.3.2
go get github.com/goccy/go-yaml@v1.15.23
```

Note: these are the latest versions as of 2026-03-15. If any fail due to transitive dependency conflicts, check `go get` output and adjust the version. The key constraint is Go 1.22+ compatibility.

- [ ] **Step 3: Verify go.mod and go.sum**

Run: `head -5 go.mod`
Expected: module line is `github.com/perigrin/git-chain`, go version is 1.22 or higher.

Run: `go mod verify`
Expected: `all modules verified`. If this fails, a dependency download was incomplete or corrupted — rerun `go get` for the failing module.

- [ ] **Step 4: Update .gitignore**

Append to `.gitignore`:
```
/git-chain
*.test
```

This excludes the built binary and Go test binaries from tracking.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum .gitignore
git commit -m "Initialize Go module with core dependencies

Cobra for CLI, go-git for ref operations, gofrs/uuid for UUIDv7,
goccy/go-yaml for frontmatter and config parsing."
```

---

## Task 2: Create main entry point and Cobra root command

**Files:**
- Create: `cmd/git-chain/main.go`
- Create: `internal/cli/root.go`
- Create: `internal/cli/root_test.go`
- Create: `internal/cli/issue.go`
- Create: `internal/cli/milestone.go`
- Create: `internal/cli/chain.go`

- [ ] **Step 1: Write the root command test**

Create `internal/cli/root_test.go`:

```go
package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/perigrin/git-chain/internal/cli"
)

func TestRootCommand_Help(t *testing.T) {
	cmd := cli.NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "refs/chain/") {
		t.Fatalf("expected help output to contain 'refs/chain/', got:\n%s", output)
	}
}

func TestRootCommand_HasIssueSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"issue"})
	if sub.Name() != "issue" {
		t.Fatalf("expected subcommand 'issue' to be registered on root command, got %q", sub.Name())
	}
}

func TestRootCommand_HasMilestoneSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"milestone"})
	if sub.Name() != "milestone" {
		t.Fatalf("expected subcommand 'milestone' to be registered on root command, got %q", sub.Name())
	}
}

func TestRootCommand_HasListSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"list"})
	if sub.Name() != "list" {
		t.Fatalf("expected subcommand 'list' to be registered on root command, got %q", sub.Name())
	}
}

func TestRootCommand_HasConfigSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"config"})
	if sub.Name() != "config" {
		t.Fatalf("expected subcommand 'config' to be registered on root command, got %q", sub.Name())
	}
}

func TestRootCommand_HasNextSubcommand(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"next"})
	if sub.Name() != "next" {
		t.Fatalf("expected subcommand 'next' to be registered on root command, got %q", sub.Name())
	}
}

func TestRootCommand_HasFormatFlag(t *testing.T) {
	cmd := cli.NewRootCommand()
	flag := cmd.PersistentFlags().Lookup("format")
	if flag == nil {
		t.Fatal("expected --format persistent flag on root command")
	}
}

func TestRootCommand_FormatFlagInheritedBySubcommands(t *testing.T) {
	cmd := cli.NewRootCommand()
	sub, _, _ := cmd.Find([]string{"issue"})
	// Cobra requires Execute() or InitDefaultHelpCmd() to propagate inherited flags.
	// Calling InheritedFlags() on a subcommand after Find works because the parent
	// is already linked during AddCommand.
	flag := sub.InheritedFlags().Lookup("format")
	if flag == nil {
		t.Fatal("expected --format flag to be inherited by 'issue' subcommand")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/cli/...`
Expected: FAIL — package and functions don't exist yet.

- [ ] **Step 3: Write the root command**

Create `internal/cli/root.go`:

```go
// ABOUTME: Cobra root command for git-chain. Wires up all subcommand groups
// ABOUTME: and persistent flags (--format). Entry point for CLI execution.
package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// NewRootCommand creates the top-level git-chain command with all subcommands.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "git-chain",
		Short: "A git-native task graph for developers and agents",
		Long: `git-chain manages development work as a dependency graph with
built-in telemetry. All state lives in git refs under refs/chain/.
No external services required.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().String("format", "", "output format (json)")

	root.AddCommand(
		NewIssueCommand(),
		NewMilestoneCommand(),
		NewListCommand(),
		NewConfigCommand(),
		NewNextCommand(),
	)

	return root
}

// Execute runs the root command. Called from main.
func Execute() {
	cmd := NewRootCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Create issue subcommand stub**

Create `internal/cli/issue.go`:

```go
// ABOUTME: Cobra command group for issue subcommands (add, list, show, edit).
// ABOUTME: Each subcommand is a factory function returning a *cobra.Command.
package cli

import "github.com/spf13/cobra"

// NewIssueCommand creates the 'issue' command group.
func NewIssueCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue",
		Short: "Manage issues in the chain",
	}

	cmd.AddCommand(
		newIssueAddCommand(),
		newIssueListCommand(),
		newIssueShowCommand(),
		newIssueEditCommand(),
	)

	return cmd
}

func newIssueAddCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add",
		Short: "Create one or more new issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("issue add: not yet implemented")
			return nil
		},
	}
}

func newIssueListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("issue list: not yet implemented")
			return nil
		},
	}
}

func newIssueShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show [ref]",
		Short: "View an issue with full context",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("issue show: not yet implemented")
			return nil
		},
	}
}

func newIssueEditCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [ref]",
		Short: "Modify an issue",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("issue edit: not yet implemented")
			return nil
		},
	}
}
```

- [ ] **Step 5: Create milestone subcommand stub**

Create `internal/cli/milestone.go`:

```go
// ABOUTME: Cobra command group for milestone subcommands (add, list, show, edit).
// ABOUTME: Each subcommand is a factory function returning a *cobra.Command.
package cli

import "github.com/spf13/cobra"

// NewMilestoneCommand creates the 'milestone' command group.
func NewMilestoneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "milestone",
		Short: "Manage milestones",
	}

	cmd.AddCommand(
		newMilestoneAddCommand(),
		newMilestoneListCommand(),
		newMilestoneShowCommand(),
		newMilestoneEditCommand(),
	)

	return cmd
}

func newMilestoneAddCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Create a new milestone",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("milestone add: not yet implemented")
			return nil
		},
	}
}

func newMilestoneListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all milestones",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("milestone list: not yet implemented")
			return nil
		},
	}
}

func newMilestoneShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show [ref]",
		Short: "Show milestone detail with issues and fever chart",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("milestone show: not yet implemented")
			return nil
		},
	}
}

func newMilestoneEditCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [ref]",
		Short: "Modify a milestone",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("milestone edit: not yet implemented")
			return nil
		},
	}
}
```

- [ ] **Step 6: Create top-level command stubs (list, config, next)**

Create `internal/cli/chain.go`:

```go
// ABOUTME: Top-level chain commands: list (show full chain), config (manage settings),
// ABOUTME: and next (alias for 'issue show HEAD' — what should I work on?).
package cli

import "github.com/spf13/cobra"

// NewListCommand creates the top-level 'list' command.
func NewListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show the full chain",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("list: not yet implemented")
			return nil
		},
	}
}

// NewConfigCommand creates the top-level 'config' command.
func NewConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config [key] [value]",
		Short: "Manage git-chain settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("config: not yet implemented")
			return nil
		},
	}
}

// NewNextCommand creates the top-level 'next' command.
func NewNextCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "next",
		Short: "Show the next issue to work on (alias for 'issue show HEAD')",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("next: not yet implemented")
			return nil
		},
	}
}
```

- [ ] **Step 7: Create main.go**

Create `cmd/git-chain/main.go`:

```go
// ABOUTME: Entry point for the git-chain binary. Git discovers this as a
// ABOUTME: subcommand when the binary is on $PATH (invoked as 'git chain').
package main

import "github.com/perigrin/git-chain/internal/cli"

func main() {
	cli.Execute()
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/cli/...`
Expected: PASS (8 tests)

- [ ] **Step 9: Verify binary compiles and runs**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go build -o git-chain ./cmd/git-chain && ./git-chain --help`
Expected: Help text showing `git-chain` with `issue`, `milestone`, `list`, `config`, and `next` subcommands.

Run: `./git-chain issue --help`
Expected: Shows `add`, `list`, `show`, `edit` subcommands.

Run: `./git-chain milestone --help`
Expected: Shows `add`, `list`, `show`, `edit` subcommands.

Clean up: `rm git-chain`

- [ ] **Step 10: Commit**

```bash
git add cmd/ internal/cli/
git commit -m "Add Cobra root command with all subcommand stubs

Root command wires up --format persistent flag, issue group (add, list,
show, edit), milestone group (add, list, show, edit), and top-level
commands (list, config, next). All print 'not yet implemented'."
```

---

## Task 3: Stub storage package

**Files:**
- Create: `internal/storage/store.go`
- Create: `internal/storage/store_test.go`

- [ ] **Step 1: Write the store test**

Create `internal/storage/store_test.go`:

```go
package storage_test

import (
	"testing"

	git "github.com/go-git/go-git/v5"
	gitstorage "github.com/go-git/go-git/v5/storage/memory"

	"github.com/perigrin/git-chain/internal/storage"
)

// This test uses a bare in-memory repo for constructor validation only.
// Write-path tests (issue 2) must use a non-bare repo with a filesystem
// worktree to match real usage via git.PlainOpen.
func TestNewStore(t *testing.T) {
	repo, err := git.Init(gitstorage.NewStorage(), nil)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	store := storage.NewStore(repo)
	if store == nil {
		t.Fatal("expected non-nil store")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/storage/...`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Write the store stub**

Create `internal/storage/store.go`:

```go
// ABOUTME: Git ref storage operations for chain entities (issues, milestones, config).
// ABOUTME: Wraps go-git to create blobs, trees, and commits on per-entity refs.
package storage

import git "github.com/go-git/go-git/v5"

// Store provides read/write access to chain entity refs in a git repository.
type Store struct {
	repo *git.Repository
}

// NewStore creates a Store backed by the given git repository.
func NewStore(repo *git.Repository) *Store {
	return &Store{repo: repo}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/storage/...`
Expected: PASS (1 test)

- [ ] **Step 5: Commit**

```bash
git add internal/storage/
git commit -m "Stub storage package with Store struct and constructor

Store wraps a go-git Repository for chain ref operations. Methods
(WriteEntity, ReadEntity, ListRefs, RefExists) added in issue 2."
```

---

## Task 4: Create App struct for dependency injection

**Files:**
- Create: `internal/cli/app.go`

Note: this task follows storage (Task 3) because app.go imports storage.

- [ ] **Step 1: Create the App struct**

Create `internal/cli/app.go`:

```go
// ABOUTME: App struct holds shared dependencies (Store, Repo) for CLI commands.
// ABOUTME: Attached to Cobra command context so subcommands can retrieve it.
package cli

import (
	"context"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-chain/internal/storage"
)

type contextKey string

const appKey contextKey = "app"

// App holds shared dependencies for all CLI commands.
type App struct {
	Store *storage.Store
	Repo  *git.Repository
}

// WithApp attaches an App to a command's context.
func WithApp(ctx context.Context, app *App) context.Context {
	return context.WithValue(ctx, appKey, app)
}

// GetApp retrieves the App from a command's context.
func GetApp(ctx context.Context) *App {
	app, _ := ctx.Value(appKey).(*App)
	return app
}
```

- [ ] **Step 2: Write app test**

Create `internal/cli/app_test.go`:

```go
package cli_test

import (
	"context"
	"testing"

	"github.com/perigrin/git-chain/internal/cli"
)

func TestGetApp_NilOnBareContext(t *testing.T) {
	app := cli.GetApp(context.Background())
	if app != nil {
		t.Fatal("expected nil App from bare context")
	}
}

func TestWithApp_RoundTrip(t *testing.T) {
	original := &cli.App{}
	ctx := cli.WithApp(context.Background(), original)
	retrieved := cli.GetApp(ctx)
	if retrieved != original {
		t.Fatal("expected GetApp to return the same App that was set with WithApp")
	}
}
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/cli/...`
Expected: PASS (10 tests — 8 from root_test.go + 2 from app_test.go)

- [ ] **Step 4: Commit**

```bash
git add internal/cli/app.go internal/cli/app_test.go
git commit -m "Add App struct for CLI dependency injection

App holds Store and Repo references, attached to command context.
Subcommands retrieve it via GetApp(). WithApp/GetApp round-trip
tested. Wiring into PersistentPreRunE deferred to issue 2."
```

---

## Task 5: Stub issue domain package

**Files:**
- Create: `internal/issue/issue.go`
- Create: `internal/issue/issue_test.go`

- [ ] **Step 1: Write the issue test**

Create `internal/issue/issue_test.go`:

```go
package issue_test

import (
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-chain/internal/issue"
)

func TestNewIssue(t *testing.T) {
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("failed to generate UUIDv7: %v", err)
	}

	iss := &issue.Issue{
		ID:        id,
		Title:     "Test issue",
		State:     issue.StatePending,
		Milestone: "v0.1",
		Created:   time.Now(),
		Updated:   time.Now(),
	}

	if iss.State != issue.StatePending {
		t.Fatalf("expected state %q, got %q", issue.StatePending, iss.State)
	}
}

func TestStateConstants(t *testing.T) {
	states := []issue.State{
		issue.StatePending,
		issue.StateInProgress,
		issue.StateDone,
		issue.StateCancelled,
	}
	expected := []string{"pending", "in-progress", "done", "cancelled"}

	for i, s := range states {
		if string(s) != expected[i] {
			t.Errorf("state %d: expected %q, got %q", i, expected[i], string(s))
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/issue/...`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Write the issue domain stub**

Create `internal/issue/issue.go`:

```go
// ABOUTME: Issue domain model for git-chain. Defines the Issue struct, state
// ABOUTME: constants, and Session type. Parsing and marshaling added in issue 2.
package issue

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// State represents the lifecycle state of an issue.
type State string

const (
	StatePending    State = "pending"
	StateInProgress State = "in-progress"
	StateDone       State = "done"
	StateCancelled  State = "cancelled"
)

// Session records a measurement window: the commit range and count between
// start/resume and pause/done transitions.
type Session struct {
	StartSHA string `yaml:"start_sha"`
	EndSHA   string `yaml:"end_sha"`
	Commits  int    `yaml:"commits"`
}

// Issue represents a node in the chain dependency graph.
type Issue struct {
	ID        uuid.UUID   `yaml:"-"`
	Title     string      `yaml:"title"`
	State     State       `yaml:"state"`
	Milestone string      `yaml:"milestone"`
	BlockedBy []uuid.UUID `yaml:"blocked_by,omitempty"`
	Blocks    []uuid.UUID `yaml:"blocks,omitempty"`
	Created   time.Time   `yaml:"created"`
	Updated   time.Time   `yaml:"updated"`
	Sessions  []Session   `yaml:"sessions,omitempty"`
	Body      string      `yaml:"-"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/issue/...`
Expected: PASS (2 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/issue/
git commit -m "Stub issue domain package with Issue struct and state constants

Issue struct defines the core fields: ID (UUIDv7), title, state,
milestone, dependencies, sessions, and markdown body. Parse/Marshal
methods added in issue 2."
```

---

## Task 6: Stub milestone domain package

**Files:**
- Create: `internal/milestone/milestone.go`
- Create: `internal/milestone/milestone_test.go`

- [ ] **Step 1: Write the milestone test**

Create `internal/milestone/milestone_test.go`:

```go
package milestone_test

import (
	"testing"
	"time"

	"github.com/perigrin/git-chain/internal/milestone"
)

func TestNewMilestone(t *testing.T) {
	ms := &milestone.Milestone{
		Name:        "v0.1",
		Description: "Initial parser implementation",
		Created:     time.Now(),
	}

	if ms.Name != "v0.1" {
		t.Fatalf("expected name %q, got %q", "v0.1", ms.Name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/milestone/...`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Write the milestone domain stub**

Create `internal/milestone/milestone.go`:

```go
// ABOUTME: Milestone domain model for git-chain. Defines the Milestone struct
// ABOUTME: as pure YAML (no markdown body). Milestones group issues for delivery.
package milestone

import "time"

// Milestone represents a delivery grouping of issues with an optional due date.
type Milestone struct {
	Name        string     `yaml:"name"`
	Due         *time.Time `yaml:"due,omitempty"`
	Description string     `yaml:"description,omitempty"`
	Resolution  string     `yaml:"resolution,omitempty"`
	Created     time.Time  `yaml:"created"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/milestone/...`
Expected: PASS (1 test)

- [ ] **Step 5: Commit**

```bash
git add internal/milestone/
git commit -m "Stub milestone domain package with Milestone struct

Milestones are pure YAML: name, optional due date, description,
and resolution command. No markdown body — metadata only."
```

---

## Task 7: Stub graph and config packages

**Prerequisite:** Task 5 (issue package) must be complete — `graph.go` imports `internal/issue`.

**Files:**
- Create: `internal/graph/graph.go`, `internal/graph/graph_test.go`
- Create: `internal/config/config.go`, `internal/config/config_test.go`

- [ ] **Step 0: Verify prerequisite**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/issue/...`
Expected: PASS. If this fails, complete Task 5 first.

- [ ] **Step 1: Write graph test**

Create `internal/graph/graph_test.go`:

```go
package graph_test

import (
	"testing"

	"github.com/perigrin/git-chain/internal/graph"
	"github.com/perigrin/git-chain/internal/issue"
)

func TestNewGraph_Empty(t *testing.T) {
	g := graph.New([]*issue.Issue{})
	if g == nil {
		t.Fatal("expected non-nil graph")
	}
}
```

- [ ] **Step 2: Write config test**

Create `internal/config/config_test.go`:

```go
package config_test

import (
	"testing"

	"github.com/perigrin/git-chain/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.Default()
	if cfg.Version != 1 {
		t.Fatalf("expected version 1, got %d", cfg.Version)
	}
	if cfg.DefaultMilestone != "v0.1" {
		t.Fatalf("expected default milestone %q, got %q", "v0.1", cfg.DefaultMilestone)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/graph/... ./internal/config/...`
Expected: FAIL — packages don't exist yet.

- [ ] **Step 4: Write graph stub**

Create `internal/graph/graph.go`:

```go
// ABOUTME: DAG construction and operations for the issue dependency graph.
// ABOUTME: Enforces graph invariants, computes critical chain and ready set.
package graph

import (
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-chain/internal/issue"
)

// Graph represents the issue dependency DAG.
type Graph struct {
	issues map[uuid.UUID]*issue.Issue
}

// New constructs a Graph from a set of issues.
func New(issues []*issue.Issue) *Graph {
	g := &Graph{
		issues: make(map[uuid.UUID]*issue.Issue, len(issues)),
	}
	for _, iss := range issues {
		g.issues[iss.ID] = iss
	}
	return g
}
```

- [ ] **Step 5: Write config stub**

Create `internal/config/config.go`:

```go
// ABOUTME: Chain configuration stored at refs/chain/_/config. Minimal for v0.1:
// ABOUTME: just version and default_milestone. Read/write via config command.
package config

// Config represents the chain configuration.
type Config struct {
	Version          int    `yaml:"version"`
	DefaultMilestone string `yaml:"default_milestone"`
}

// Default returns the default configuration for a new chain.
func Default() *Config {
	return &Config{
		Version:          1,
		DefaultMilestone: "v0.1",
	}
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/graph/... ./internal/config/...`
Expected: PASS (2 tests)

- [ ] **Step 7: Commit**

```bash
git add internal/graph/ internal/config/
git commit -m "Stub graph and config packages

Graph uses uuid.UUID keys for the issue map. Config provides Default()
returning version 1 with v0.1 milestone. Full implementation in later issues."
```

---

## Task 8: Stub telemetry and resolve packages

**Files:**
- Create: `internal/telemetry/telemetry.go`, `internal/telemetry/telemetry_test.go`
- Create: `internal/resolve/resolve.go`, `internal/resolve/resolve_test.go`

- [ ] **Step 1: Write telemetry test**

Create `internal/telemetry/telemetry_test.go`:

```go
package telemetry_test

import (
	"testing"

	"github.com/perigrin/git-chain/internal/telemetry"
)

func TestStatusConstants(t *testing.T) {
	if telemetry.StatusGreen != "GREEN" {
		t.Fatalf("expected %q, got %q", "GREEN", telemetry.StatusGreen)
	}
	if telemetry.StatusYellow != "YELLOW" {
		t.Fatalf("expected %q, got %q", "YELLOW", telemetry.StatusYellow)
	}
	if telemetry.StatusRed != "RED" {
		t.Fatalf("expected %q, got %q", "RED", telemetry.StatusRed)
	}
}

func TestStats_ZeroValue(t *testing.T) {
	s := telemetry.Stats{}
	if s.FeverStatus != "" {
		t.Fatalf("expected zero-value FeverStatus to be empty, got %q", s.FeverStatus)
	}
}
```

- [ ] **Step 2: Write resolve test**

Create `internal/resolve/resolve_test.go`:

```go
package resolve_test

import (
	"testing"

	"github.com/perigrin/git-chain/internal/resolve"
)

func TestIsHead(t *testing.T) {
	if !resolve.IsHead("HEAD") {
		t.Fatal("expected 'HEAD' to be recognized as HEAD")
	}
	if !resolve.IsHead("") {
		t.Fatal("expected empty string to be recognized as HEAD (no explicit target defaults to HEAD)")
	}
	if resolve.IsHead("019444a1") {
		t.Fatal("expected UUID prefix not to be recognized as HEAD")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/telemetry/... ./internal/resolve/...`
Expected: FAIL — packages don't exist yet.

- [ ] **Step 4: Write telemetry stub**

Create `internal/telemetry/telemetry.go`:

```go
// ABOUTME: Telemetry computation for development signals (MPG, speed, buffer).
// ABOUTME: Stateless — derives all indicators from issue and milestone data.
package telemetry

// Status represents the fever chart health status of a milestone.
type Status string

const (
	StatusGreen  Status = "GREEN"
	StatusYellow Status = "YELLOW"
	StatusRed    Status = "RED"
)

// Stats holds the derived telemetry indicators for a milestone.
type Stats struct {
	MPG          float64
	Speed        float64
	BufferTotal  float64
	BufferBurned float64
	TimeInChain  float64
	FeverStatus  Status
}
```

- [ ] **Step 5: Write resolve stub**

Create `internal/resolve/resolve.go`:

```go
// ABOUTME: Ref argument resolution for CLI commands. Resolves user input
// ABOUTME: (HEAD, tag, UUID prefix, title substring) to an entity ref path.
package resolve

// IsHead returns true if the input resolves to the HEAD reference.
// An empty ref argument means the caller passed no explicit target,
// which resolves to HEAD — the current in-progress issue or next on
// the critical chain.
func IsHead(input string) bool {
	return input == "" || input == "HEAD"
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./internal/telemetry/... ./internal/resolve/...`
Expected: PASS (3 tests — 2 telemetry + 1 resolve)

- [ ] **Step 7: Commit**

```bash
git add internal/telemetry/ internal/resolve/
git commit -m "Stub telemetry and resolve packages

Telemetry defines Stats and fever chart Status constants. Resolve
provides IsHead() for ref argument detection. Full implementation
in later issues."
```

---

## Task 9: Run full test suite and verify binary

- [ ] **Step 1: Run all tests and vet**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go vet ./...`
Expected: no output (clean).

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go test ./...`
Expected: PASS — all packages, all tests green.

- [ ] **Step 2: Build and verify binary**

Run: `cd /home/perigrin/dev/git-chain/.worktrees/bootstrap && go build -o git-chain ./cmd/git-chain && ./git-chain --help`
Expected: Help output showing `git-chain` with `issue`, `milestone`, `list`, `config`, and `next` subcommands.

- [ ] **Step 3: Verify subcommand help**

Run: `./git-chain issue --help`
Expected: Shows `add`, `list`, `show`, `edit` subcommands.

Run: `./git-chain milestone --help`
Expected: Shows `add`, `list`, `show`, `edit` subcommands.

- [ ] **Step 4: Clean up binary**

Run: `rm git-chain`

- [ ] **Step 5: Final verification — no unexpected untracked files**

Run: `git status`
Expected: clean working tree, nothing untracked (binary excluded by .gitignore).

If any test failure was found during this task, fix the issue and create a commit with message `fix: <description of what was wrong>`.
