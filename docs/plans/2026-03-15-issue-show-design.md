# Issue 3: `issue show` — Design

## Scope

Read path for a single issue: UUID prefix resolution, HEAD resolution (pre-graph placeholder), human-readable display, `--format json` with structured body section parsing (Prerequisites, Context, Acceptance Criteria).

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Resolution scope | UUID prefix + HEAD | Full resolve chain (tags, title) deferred to issue 10 |
| HEAD behavior | First in-progress, else first pending by UUID sort | Pre-graph placeholder; issue 7 replaces with critical chain |
| Body parsing | Full section extraction | Prerequisites, Context, AC parsed into structured JSON per PRD |
| Section parser location | internal/issue/sections.go | Presentation concern, separate from core domain model |
| Display logic location | internal/cli/issue_show.go | Keeps issue.go from growing; owns both human and JSON output |

## Resolution

`resolve.ResolveRef(store, input) (string, error)` — resolves user input to a full ref path.

Order: HEAD (empty or "HEAD") → UUID prefix scan. Ambiguous prefix returns error with candidates. HEAD resolves to first in-progress issue, or first pending issue by lexicographic UUID order (approximates creation time via UUIDv7).

## Body Section Parsing

`issue.ParseSections(body string) *Sections` — extracts structured data from markdown body.

Splits on `## ` headings. Matches "Prerequisites", "Context", "Acceptance Criteria" case-insensitively. Everything else becomes description.

- **Prerequisites / Acceptance Criteria**: `- [x]` / `- [ ]` checkbox extraction
- **Context**: `- paths:`, `- docs:`, `- commands:`, `- entrypoints:` key-value extraction
- **Description**: non-section body text

Loose parsing — malformed sections produce empty fields, not errors.

## Output

**Human-readable**: Title, state, milestone, created date, blocked by/blocks (with titles from loaded deps), raw body sections, formatted naturally.

**JSON**: Presentation struct combining Issue metadata with parsed Sections. Includes both raw `body` and structured fields.

## Testing

**Resolve** (6 tests): UUID prefix match/ambiguous/not-found, HEAD with in-progress/pending/empty.

**Sections** (5 tests): Full parse, checkbox extraction, context extraction, empty body, no-sections body.

**CLI** (4 tests): Show by UUID prefix, show HEAD, JSON output with sections, not-found error.
