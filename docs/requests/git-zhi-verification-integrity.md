---
stability: 1
covers:
  - internal/verify
  - internal/docs
  - internal/cli/chain_list.go
---

# Verification integrity in git-zhi

A design document for git-zhi, written from crochet. It is the deliverable of
`rfc-0003` issue 12, and it is meant to be taken into git-zhi's own tree and run
through the crochet loop there — this document is the subject of that
assessment, not its outcome.

**Every gap below was observed on git-zhi 0.6.0**, in this repository or in a
scratch repository, with the command and its output recorded. No line numbers
are cited into git-zhi's tree: nothing here can check them, and three separate
passes over `0002-worker-identity.md` found stale cross-repository citations.

Gaps 1 and 5 were re-derived against the code during review in git-zhi's tree:
the observations held, the diagnoses did not, and both now describe the
mechanism that actually produces them. Paragraphs marked *decided in review*
record choices this document originally left open.

## Problem Statement

Six defects, five of them one shape: **a check that reports the result it would
report if clean, when what it actually examined was empty or wrong.**

Crochet's whole protocol rests on gates leaving evidence. A gate whose green and
whose vacuum are indistinguishable is not evidence, and it is worse than an
absent gate, because an absent gate is visibly absent.

### 1. The acceptance-criteria parser drops criteria written under a bare heading

**The most serious of the six, and the one to fix first.**

`git zhi verify <milestone>` has never run an acceptance criterion in `rfc-0003`.
Every block it prints is labelled `Negative:`. `issue show --format json` returns
the negative scenarios in **both** the `acceptance_criteria` and
`negative_scenarios` arrays — identical to each other, and both differing from
the `## Acceptance Criteria` list in the body markdown, which holds the real
criteria.

The cause is the body parser, not the field plumbing. Two probe issues isolate
it:

| issue body shape | `acceptance_criteria` | `negative_scenarios` |
|---|---|---|
| `### Positive Scenarios` + `### Negative Scenarios` under the H2 | the positive one | the negative one |
| criteria directly under `## Acceptance Criteria`, then `### Negative Scenarios` | **the negative one** | the negative one |

Criteria are collected only when they sit under an explicit
`### Positive Scenarios` heading. Written directly under the `## Acceptance
Criteria` H2 with any H3 below them, they are dropped entirely and
`acceptance_criteria` falls back to the negatives — which is why the two arrays
come back identical and why the real criterion never runs.

So `milestone edit --state complete` runs the closing gate over a field that
never contains an acceptance criterion. It reports a count and passes; what it
checked was the negative scenarios, twice.

Observed across three issues in `rfc-0003`. The real criteria were then run by
hand and do pass — so this is a defect in the gate, not in the work, which is the
worse of the two. It means a milestone's own verification is not evidence about
the milestone.

**Fixed means:** criteria written directly under `## Acceptance Criteria` are
extracted as positive scenarios. The two arrays then hold different content, and
`verify` reports a count that matches the `## Acceptance Criteria` list.

### 2. `verify` reads only done issues, so `--dry-run` is inert when it is needed

`skills/chain-review/chain-review.md` runs `verify --dry-run` in list-only mode
to confirm every criterion is extractable and runnable **before** execution
starts. That is the only state chain-review ever runs in, and in it every issue
is pending.

Observed: `git zhi verify rfc-0003 --dry-run` against twelve pending issues
carrying sixty-nine paren-wrapped commands returns empty, exit 0.

The check passes because there was nothing to check. This is the vacuous pass in
its purest form: the gate designed to catch unrunnable criteria before execution
cannot see a single criterion at the moment it runs.

**Fixed means:** `--dry-run` extracts regardless of issue state.

### 3. An empty subject reports success

The general form of 2, and worth fixing separately, because fixing 2 alone still
leaves a `verify` over a milestone whose issues carry no criteria reporting
green.

git-zhi's own proposal — putting it in `ExtractCommands`' caller rather than in
`--dry-run` — is **better than what crochet originally asked for**, and crochet
withdraws its narrower version. A real `verify` that found nothing should say so
on stderr and exit non-zero, not only a `--dry-run` that found nothing.

**Fixed means:** extracting zero commands is reported as such and exits non-zero,
wherever it happens.

**Scope, decided in review.** "Wherever it happens" reaches further than it
first appears: `runMilestoneComplete`'s third gate shells out to
`git-zhi verify <milestone>` and fails the transition on any non-zero exit. The
gate therefore inherits this rule, and a milestone whose issues carry no
runnable criteria can no longer be closed. That is the intended outcome — it is
the closing gate this request exists to make meaningful. A milestone that is
deliberately criteria-free needs an explicit override on `milestone edit` to
close, so the exemption is a typed act and not a silent pass.

### 4. `verify` ignores the milestone body

0003 says a milestone carries the decision's acceptance criteria, and that the
review gate verifies them. `verify` extracts only from issues, so milestone-level
criteria are unverifiable and the review gate would read them by eye.

Additive: keep issue extraction exactly as it is. The negative scenario crochet
would hold this to is that milestone-body extraction does not lose issue-body
extraction — a dry run over `rfc-0001` must still name `xt/run.sh`.

**Output shape, decided in review.** `verify`'s rows are keyed by issue title —
`<issue title>  <kind>  <command>` — and a milestone-body criterion has no issue
to name in that column. Body criteria emit as their own section above the issue
rows rather than being folded in as pseudo-titled rows, so a reader and a parser
can both tell milestone-level criteria from issue-level ones without inferring
it from a title that happens to match the milestone's name.

**Fixed means:** criteria in the milestone body are extracted and run alongside
the issues', reported in their own section ahead of the issue rows.

### 5. `docs check` reports reachability it did not examine

`git zhi docs check` prints `✓ All files reachable from CONTRIBUTING.md` in a
repository where it examined nothing. Observed in a scratch repository with
`docs/decisions/` and no contributing file: all green, exit 0.

The missing `CONTRIBUTING.md` is not the cause, and the diagnosis matters because
it changes the fix. Four fixtures against 0.6.0:

| `docs/` contents | `CONTRIBUTING.md` | result |
|---|---|---|
| `docs/decisions/0001-a.md` only | absent | `✓ All files reachable` — exit 0 |
| `docs/decisions/0001-a.md` only | present, links nothing | `✓ All files reachable` — exit 0 |
| `docs/guides/g.md` | absent | `✗ 1 unreachable file(s)` — exit 1 |
| `docs/guides/g.md` | present, links nothing | `✗ 1 unreachable file(s)` — exit 1 |

A missing root already produces a loud red: the BFS starts at `CONTRIBUTING.md`,
the read fails, nothing is reachable, and everything is reported. The green comes
from the early return in `checkReachability` when the set of files to check is
empty — every file was under a reachability-exempt prefix, so the walk collected
nothing and the function returned before the BFS ran. That is why the root's
presence makes no difference to the first two rows.

This is gap 3's shape applied to a second check, and it takes gap 3's rule rather
than one of its own. Same shape as the `docs health` behaviour crochet already
documents for its own contributors — "its summary reports three zeros both when
nothing has drifted and when it is observing no documents at all". Here it
matters more, because `crochet:assess` depends on this check to catch an
unreachable archive.

A smaller asymmetry surfaced alongside it, and it is a separate ask:
`docs/decisions/` gets an implicit reachability pass and `docs/assessments/` does
not. The exempt list is a package-level variable in `internal/docs`; whatever the
rule is, it is not stated anywhere a skill author would find it.

**Fixed means:** a reachability check that examined zero files reports that it
examined zero files, rather than claiming everything is reachable. Separately,
the reachability exemptions are documented where a skill author will find them.

### 6. `git zhi list --all` is a silent no-op

New, and not previously reported. git-zhi asked whether anything of crochet's
uses the top-level `list --all`; the answer is that crochet's **documentation**
does, which is worse than a call site, because it ships the wrong claim to every
agent that reads it.

Repro, in this repository, today. Counting `"id"` keys in each JSON output:

| command | issues returned |
|---|---|
| top-level `list --all` | 1 |
| `issue list --all` | 40 |

The two `list` forms, with and without `--all`, are byte-identical. `--all` exits
0 there; an unrecognised flag such as `--bogusflag` exits 1.

**And `--help` advertises it.** `git zhi list --help` prints:

```
      --all                include done and cancelled issues
```

So this is not an unknown flag being tolerated, and not an undocumented one
either. It is a documented, recognised flag that does nothing, which no caller
can detect and which `--help` actively denies. crochet's reference tells agents
that when `--help` and the reference disagree, `--help` wins; this is the first
known case where that rule produces the wrong answer, and the reference now
carries it as a counterexample.

crochet's own defect from this: `skills/how-to-use-git-zhi/how-to-use-git-zhi.md`
lists `--all` in the flag set for the top-level `list`, implying the `issue list`
meaning. Crochet will correct that regardless of what git-zhi decides.

**Fixed means:** either `--all` includes done issues on the top-level `list`, or
it is rejected there. Silently accepting it is the only outcome crochet cannot
work with.

## What crochet also wants, smaller

**`issue edit` has no `--title`.** Retitling an issue means deleting and
recreating it, which loses its id — and the id is what the milestone checklist,
the `blocks` graph and every skipped-gate record refer to. Observed while
correcting two issue titles in `rfc-0003`; both corrections were abandoned rather
than break the references.

## Questions git-zhi asked, answered

**`historian` never sets `Milestone` — is it load-bearing for the postmortem?**
No. `skills/postmortem/postmortem.md` names its data sources as issue list,
milestone telemetry, verify results, docs health and the sanbao report; it never
invokes `historian`. crochet does not import reconstructed history anywhere.
Treat it as theoretical for crochet and decide it on git-zhi's own grounds.

**Does crochet want whole-repo `sanbao`?** Not today — the postmortem is scoped
to a milestone by construction. The case where crochet would want it: a
cross-milestone trend, since 0003's postmortem asks whether a *way of working* is
holding up, and one milestone cannot answer that. That is a real future want and
not a current blocker.

**Does anything use top-level `list --all`?** See gap 6. No call site; one
documentation site, being corrected.

**The seven orphaned `git-zhi-<plugin>` symlinks.** Nothing of crochet's probes
them. Every remaining `git-zhi-<name>` string under `skills/` is prose naming
verify's extraction contract, never a command in command position — checked
across the whole skills tree. This is perigrin's cleanup now, not crochet's
dependency. crochet keeps `t/git-zhi-subcommands.sh`, which exists because one of
those symlinks stopped dispatching in 0.5.0 while its `--help` kept exiting 0.

**The line-number convention.** crochet keeps it either way; it is enforced by
`xt/run.sh` against `docs/decisions/` here. git-zhi should decide it on its own
grounds.

## Decided in review: git-zhi's own changes

Neither of these is crochet's ask. Both came out of assessing this document in
git-zhi's tree, and both exist because gap 3's rule is only safe once empty
milestones stop appearing on their own.

**Empty milestones are no longer manufactured.** `EnsureInitialized` eagerly
created `refs/zhi/_/milestones/<default>` on the first git-zhi command in any
repository, so every repository shipped with a zero-issue milestone nobody
wrote — a guaranteed vacuous verification subject, and the source of the `0/0`
case that makes gap 3 look like a trap. That eager creation is dropped. The
default milestone name stays in config, because it is what `issue add` stamps on
an issue when `--milestone` is absent, and the milestone ref is created lazily
when the first issue names it.

The invariant this buys: **a milestone ref exists only if an issue names it, or a
human ran `milestone add`.** An empty milestone is now always deliberate, which
is what makes gap 3 refusing to close one the correct behaviour rather than an
obstacle.

**`git zhi milestone prune`** disposes of the ones already in the wild. It
removes a milestone only when it has zero issues *and* no authored content — no
body, no resolution, no postmortem, no due date, not completed, not tagged. That
predicate matches exactly what `EnsureInitialized` used to manufacture and
nothing a human typed, so a milestone created deliberately and not yet populated
survives. It skips tagged milestones rather than leaving a tag ref pointing at
nothing, carries `--dry-run`, and reports what it examined and not only what it
removed.

## Found while closing v0.4.1-crochet-parity

Three defects surfaced while giving that milestone's acceptance criteria real
tests. None of them is crochet's ask, and none was known when this document was
written; they are recorded here because the chain that implements this document
carries them, and a spec that does not describe the work is the same defect this
document is about.

**`issue edit --body -` hangs forever when stdin is a terminal.** The read is an
unconditional `io.ReadAll` with no interactivity check, and there is no TTY
detection anywhere in `internal/cli`. The same shape serves `milestone edit`'s
`--body`, `--resolution` and `--postmortem`, so one guard in a shared helper
covers four flags.

**`milestone edit --body -` exits 0 when stdin delivers nothing.** The field is
correctly left alone, but the command reports success for an invocation that did
nothing, and the explanatory notice goes to stdout where no caller checks it.
This one does change an existing default, deliberately: it is gap 3's shape in
the write path, and the exit code is the only thing being changed.

**`verify` extracts the first parenthesised backtick on a criterion, not the
last.** `parenBacktickRe`'s own comment says the command is "typically the last
parenthetical on an AC line" and `FindStringSubmatch` returns the first, so a
criterion mentioning any parenthesised code inline has its real command
shadowed. The gate then reports on something the author never asked it to run.

## What crochet is not asking for

No new `--format json` surfaces, no new subcommands, and nothing that changes an
existing default — crochet's own asks stay inside those bounds. The changes in
the two sections above are git-zhi's, adopted on its own grounds, and one of
them does change a default. Five of the six gaps above are a check learning to
distinguish "nothing was wrong" from "nothing was examined", and that
distinction is the entire request.

## Coordination

The mechanism between the two repositories already exists:
`git_zhi_min_version` in `.claude-plugin/plugin.json`, which `crochet:preflight`
enforces. Raise it once these ship, so a crochet depending on them cannot report
a healthy environment to an agent whose skills cannot work in it.

Priority, if it must be ordered: **1, then 2 and 3 together, then 4.** Gap 1
means no milestone in either repository is currently verified by its own gate.
