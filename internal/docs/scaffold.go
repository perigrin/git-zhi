// ABOUTME: Scaffold creates the canonical docs/ directory structure and living
// ABOUTME: documents, and maintains the Short Links section in CONTRIBUTING.md.
package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// canonicalDirs is the set of directories Scaffold creates under repoRoot.
var canonicalDirs = []string{
	"docs/plans",
	"docs/decisions",
	"docs/contributing",
	"docs/guides",
	"docs/architecture",
	"docs/reference",
}

// shortLinksHeading is the exact heading used to identify the Short Links
// section in CONTRIBUTING.md. Scaffold checks for this string to avoid
// duplicating the section.
const shortLinksHeading = "## Short Links to Important Resources"

// shortLinksSection is the full Short Links section appended to
// CONTRIBUTING.md when no such section exists.
const shortLinksSection = `
## Short Links to Important Resources

- [Plans](docs/plans) — implementation plans and roadmap documents
- [Decisions](docs/decisions) — architecture decision records (ADRs)
- [Contributing guides](docs/contributing) — coding conventions and development workflow
- [Guides](docs/guides) — how-to guides for common tasks
- [Architecture](docs/architecture) — system design and component descriptions
- [Reference](docs/reference) — reference documentation
`

// codingConventionsContent is the starter content for
// docs/contributing/coding-conventions.md.
const codingConventionsContent = `---
stability: 1
covers: []
---

## Code Style

Follow the conventions of the surrounding code. Consistency within a file
matters more than strict adherence to an external style guide.

## Naming

Use clear, descriptive names. Avoid abbreviations unless they are universally
understood in context.

## Error Handling

Return errors rather than panicking. Wrap errors with context using
fmt.Errorf("context: %w", err).

## Testing

Write a failing test before implementation (TDD). Tests use real data — no
mocks or stubs.
`

// developmentWorkflowContent is the starter content for
// docs/contributing/development-workflow.md.
const developmentWorkflowContent = `---
stability: 1
covers: []
---

## Setup

Install Go 1.24+, git, and clone the repository. Run ` + "`go build -o git-zhi ./cmd/git-zhi/`" + `
to verify the build.

## Branching

Create feature branches from ` + "`pu`" + `. Merge back to ` + "`pu`" + ` via pull request.

## Review

All changes require a pull request. Tests must pass. No ` + "`--no-verify`" + `.

## Deploy

Build the binaries with ` + "`go build`" + ` and place them on ` + "`$PATH`" + `.
`

// hubContributingContent is the full hub-style CONTRIBUTING.md created when
// no CONTRIBUTING.md exists in the repository.
func hubContributingContent() string {
	return "# Contributing\n" + shortLinksSection
}

// Scaffold creates the canonical docs/ directory structure under repoRoot.
// It is idempotent: existing directories and files are left untouched.
// Living documents receive starter content on first creation. CONTRIBUTING.md
// is created as a hub-style document if absent; if it exists, a Short Links
// section is appended unless one is already present.
func Scaffold(repoRoot string) error {
	if err := createDirs(repoRoot); err != nil {
		return err
	}
	if err := createLivingDocs(repoRoot); err != nil {
		return err
	}
	if err := ensureContributing(repoRoot); err != nil {
		return err
	}
	return nil
}

// createDirs creates any missing canonical directories under repoRoot.
func createDirs(repoRoot string) error {
	for _, dir := range canonicalDirs {
		full := filepath.Join(repoRoot, filepath.FromSlash(dir))
		if err := os.MkdirAll(full, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}
	return nil
}

// livingDoc describes a document that Scaffold creates with starter content.
type livingDoc struct {
	path    string
	content string
}

// livingDocs lists the living documents Scaffold manages.
var livingDocs = []livingDoc{
	{"docs/contributing/coding-conventions.md", codingConventionsContent},
	{"docs/contributing/development-workflow.md", developmentWorkflowContent},
}

// createLivingDocs writes starter content for each living document that does
// not yet exist. Existing files are never modified.
func createLivingDocs(repoRoot string) error {
	for _, doc := range livingDocs {
		full := filepath.Join(repoRoot, filepath.FromSlash(doc.path))
		if _, err := os.Stat(full); err == nil {
			// File already exists — leave it alone.
			continue
		}
		if err := os.WriteFile(full, []byte(doc.content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", doc.path, err)
		}
	}
	return nil
}

// ensureContributing creates CONTRIBUTING.md if it does not exist, or appends
// a Short Links section if the file exists but lacks one.
func ensureContributing(repoRoot string) error {
	path := filepath.Join(repoRoot, "CONTRIBUTING.md")
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		// No CONTRIBUTING.md — create the hub-style document.
		return os.WriteFile(path, []byte(hubContributingContent()), 0o644)
	}
	if err != nil {
		return fmt.Errorf("stat CONTRIBUTING.md: %w", err)
	}

	// File exists — append Short Links section if it is absent.
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read CONTRIBUTING.md: %w", err)
	}
	if strings.Contains(string(raw), shortLinksHeading) {
		// Section already present — nothing to do.
		return nil
	}

	appended := string(raw) + shortLinksSection
	return os.WriteFile(path, []byte(appended), 0o644)
}
