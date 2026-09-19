# 0002. Historian output is measurement, not work

Date: 2026-09-19

## Status

Proposed

## Context

`git zhi historian` walks the git log, clusters commits, and writes the result
into `refs/zhi/_/issues/` as issues in state `done`. Running it against this
repository produced twelve issues from 305 commits, at full coverage.

Those twelve are not work items in any sense the rest of the tool recognises.
They arrive `done`, they block nothing, nothing blocks them, and `git zhi next`
will never surface one. Nor are they decision records, which was the first
alternative considered. Here is a complete imported issue, with nothing elided:

    title:          "Remove self-update subsystem"
    state:          done
    milestone:      ""
    sessions:       [{start_sha: 2814a15, end_sha: 42cfa8e, commits: 7}]
    transitions:    [{state, actor: "human:Chris Prather", timestamp} x2]
    observed_paths: [CLAUDE.md, cmd/git-zhi/main.go, internal/cli/discover.go, ...]
    source:         cluster
    confidence:     0.55

There is no body. No context, no rationale, no prose of any kind — the title is
the only human-readable field, and for five of the twelve the title is `#3`,
`#9`, `#16`, `#17` or `#23`, being the ticket reference the commit subject
carried. A commit range contains no reasoning to reconstruct, so this could not
be made into a decision record by any amount of formatting.

What it is, is measurement: a time-boxed window of commits attributed to an
actor, with the paths touched. That is exactly the shape DORA and SPACE
metrics compute over, and exactly what lineage needs to map blame back to an
issue. The data is right. Only its filing is in question.

Two facts bound the problem, and both cut against an expensive answer.

The dilution is narrower than it first appears. `git zhi list` excludes done
issues by default, so the twelve are already invisible in the default view;
this repository's list shows its four real issues and nothing else. They
surface only under `--all` or `--state done`.

And the discriminator already exists and is already correct. `Source` is set on
every historian issue and, being `omitempty`, is absent on every hand-written
one. The split in this repository is exact: four issues with no source, twelve
with `cluster`, `tracker-match`, `single-commit` or `ticket-ref`.

Against that, one thing is actually broken. Historian never assigns a
milestone — `grep -rn Milestone internal/historian/` returns nothing. `issue
add` applies the configured default; historian writes to the store directly and
bypasses it. So a 305-of-305 import is invisible to every milestone-scoped
view: `milestone show`, progress, forecasting, and `sanbao`, which reported
`n/a` lead time and `0%` metric completeness for a repository that had just
imported its entire history. The telemetry these records exist to feed cannot
see them.

That is the third time this week a sensor has reported zeros while observing
nothing, after `docs check` passing over files it could not reach and
`docs health` reporting no drift across documents it was not watching.

## Decision

Proposed, not yet accepted. Three options, in ascending cost.

**A. Assign a milestone; leave the namespace alone.** Historian assigns its
issues to a milestone of its own rather than to the configured default, which
would otherwise drop twelve historical records into whatever milestone is
current and wreck its progress count. Telemetry can then see them. Nothing else
changes, and the default view is already clean.

**B. A, plus filter human-facing views on `Source`.** `list`, `next` and the
ready set exclude issues carrying a source unless asked for them. Cheap,
because the field is already populated. The risk is precisely the defect fixed
in `list --milestone`: a filter that must be re-applied in several places is a
filter that will be forgotten in one of them, and the failure is silent.

**C. A separate ref namespace for historian output.** The cleanest separation,
and the most expensive: `LoadAllIssues` is the shared loader for every consumer,
so telemetry and lineage would need to read both namespaces while the graph and
the work views read one. It also needs a migration for imports already written.

The recommendation is A now, with B held in reserve until a view is observed to
be diluted in practice rather than in principle. C buys separation the `Source`
field already provides, at the cost of a second loader.

Whichever is chosen, the titles are a separate defect. A tracker match that
yields `#17` has discarded the commit subject that would have made the record
legible, and is a worse record than no match at all.

## Consequences

Under A, historian output stays in the issues namespace and is distinguished by
data rather than by location. Anything that needs to tell the two apart can,
because `Source` already says so; anything that does not care continues to read
one namespace through one loader.

The cost is that "issue" keeps meaning two things — a unit of intended work and
a record of completed work — held apart by a field rather than by structure. If
that ambiguity starts producing bugs rather than just discomfort, B and then C
remain available, and this decision should be superseded rather than amended.

Deferring C has a migration cost that grows with every import: moving records
later means rewriting refs that telemetry has already computed over.
