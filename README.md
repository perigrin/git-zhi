# git-zhi

A git-native task graph that tells developers — and agents — what to do next.

git-zhi (織, "the loom") manages development work as a dependency graph with
built-in telemetry. All state lives in git refs. No external services, no API
tokens, works offline, syncs when you push.

## Quick Start

```bash
go install github.com/perigrin/git-zhi@latest

cd your-project
git zhi issue add <<'EOF'
---
title: "Implement the parser"
---

## Context

- paths: lib/Parser.pm
- commands: prove -lv t/parser/

## Acceptance Criteria

- [ ] basic expressions parse
- [ ] error messages include line numbers
EOF

git zhi issue show
git zhi issue edit lexer --state start
# do your work, commit as normal
git zhi issue edit lexer --state done
git zhi milestone show
```

An agent does the same thing:

```bash
git zhi issue show --format json   # structured context, ready to work
git zhi issue edit <id> --state start
# agent works using context.paths, context.commands, acceptance_criteria
git zhi issue edit <id> --state done
```

v0.2 adds quality gates, parallel scheduling, and engineering metrics:

```bash
# Quality gates
git zhi milestone edit v0.1 --resolve     # run resolution command
git zhi verify v0.1                       # check all acceptance criteria
git zhi milestone edit v0.1 --state complete  # gated on all checks passing

# Parallelization
git zhi list --ready                      # what can run in parallel?
git zhi next --actor "agent:claude-code"  # per-worker scheduling

# Engineering metrics
git zhi sanbao report v0.1               # DORA, SPACE, sentiment, complexity

# Documentation health
git zhi docs health                       # churn-relative staleness detection
```

## Install

Requires **Go 1.24+** and **git**.

```bash
go install github.com/perigrin/git-zhi@latest
```

Or build from source:

```bash
git clone https://github.com/perigrin/git-zhi.git
cd git-zhi
go build -o git-zhi ./cmd/git-zhi/
go build -o git-zhi-verify ./cmd/git-zhi-verify/
go build -o git-zhi-sanbao ./cmd/git-zhi-sanbao/
go build -o git-zhi-docs ./cmd/git-zhi-docs/
```

Place `git-zhi` on your `$PATH`. Git discovers it automatically — `git zhi`
just works.

## Commands

**Issues**

```bash
git zhi issue add                        # create from stdin (--- for batch)
git zhi issue list                       # all open issues
git zhi issue list --state pending       # filter by state
git zhi issue show <ref>                 # detail view (or HEAD if omitted)
git zhi issue show --format json         # structured output for agents
git zhi issue edit <ref> --state start   # start working (records HEAD sha)
git zhi issue edit <ref> --state pause   # pause (closes measurement window)
git zhi issue edit <ref> --state done    # complete (closes final window)
git zhi issue edit <ref> --block <other> # add dependency edge
git zhi issue edit <ref> --tag parser    # lightweight named reference
git zhi issue edit <ref> --split         # split into multiple issues (stdin)
git zhi issue edit <ref> --merge <other> # combine two issues
git zhi issue edit <ref> --purge --yes   # permanently delete
```

**Milestones**

```bash
git zhi milestone add v0.2 --due 2026-05-01
git zhi milestone list
git zhi milestone show                   # current milestone with telemetry
git zhi milestone show --workers 3       # completion forecast with worker count
git zhi milestone edit v0.1 --name "Parser MVP"
git zhi milestone edit v0.1 --resolve    # execute resolution command
git zhi milestone edit v0.1 --state complete  # gated completion
```

**Quality Gates**

```bash
git zhi milestone edit <name> --resolve          # execute resolution command
git zhi milestone edit <name> --state complete   # gated completion
```

**Chain**

```bash
git zhi list                             # topological sort, grouped by milestone
git zhi list --critical                  # critical chain + parallel work
git zhi list --ready                     # parallel-safe ready set with path analysis
git zhi next                             # what should I work on? (alias: issue show HEAD)
git zhi next --actor <identity>          # per-worker HEAD resolution
git zhi config                           # display configuration
git zhi config default_milestone v0.2    # set default
```

**Verify Plugin**

```bash
git zhi verify <milestone>               # run all AC commands
git zhi verify <milestone> --fail-fast   # stop on first failure
git zhi verify <milestone> --dry-run     # list commands without running
git zhi verify <milestone> --format json # structured results
```

**Sanbao Observatory**

```bash
git zhi sanbao report <milestone>                  # full engineering report
git zhi sanbao report <milestone> --domain dora    # DORA metrics only
git zhi sanbao report <milestone> --format json
```

**Documentation Health**

```bash
git zhi docs init                        # scaffold canonical docs/ structure
git zhi docs check                       # validate structure and links
git zhi docs health                      # churn-relative staleness detection
```

## How It Works

State is stored as per-entity git refs under `refs/zhi/`. Each issue gets its
own ref whose commit history is an append-only event log. No mutable state
files, no databases — just git objects and refs.

```
refs/zhi/_/issues/019444a1-...      # one ref per issue
refs/zhi/_/milestones/v0.1          # one ref per milestone
refs/zhi/_/config                   # chain configuration
refs/zhi/_/tags/parser              # lightweight named references
```

Two developers editing different issues touch different refs — zero merge
conflicts. `git push` and `git pull` sync everything.

## Telemetry

git-zhi treats the repository as a sensor. It observes activity signals and
produces derived indicators:

- **MPG** — commits per issue. Are your issues right-sized?
- **Speed** — issues per week. When will you be done?
- **Buffer** — reserve capacity. Are you ahead or behind?
- **Fever chart** — GREEN/YELLOW/RED health at a glance.
- **Time-in-chain** — ratio of focused work to calendar time.
- **Forecast accuracy** — predicted vs. actual delivery. Is your model improving?
- **Ready set width** — how many issues are unblocked at any moment.
- **Parallel efficiency** — fraction of capacity running concurrently.
- **Lineage depth** — dependency chain length from root to leaf.

Nobody enters estimates. The tool watches what happens and projects forward.

## Extending

Any executable named `git-zhi-<name>` on `$PATH` is invocable as
`git zhi <name>`. Write plugins in any language. Three bundled plugins
(`git-zhi-verify`, `git-zhi-sanbao`, `git-zhi-docs`) demonstrate the
pattern and cover quality gates, engineering metrics, and documentation
health out of the box.

## Design

git-zhi adapts Critical Chain Project Management (Goldratt) to software
development. Tasks are sized for ideal conditions. Uncertainty pools into
shared milestone buffers. The dependency graph determines what is ready, what
is critical, and what to work on next.

For the full design, see [docs/PRD/](docs/PRD/).

## Contributing

See [CONTRIBUTORS.md](CONTRIBUTORS.md) for development setup, conventions,
and how to contribute.

## License

MIT
