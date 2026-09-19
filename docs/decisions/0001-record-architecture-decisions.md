# 0001. Record architecture decisions

Date: 2026-09-19

## Status

Accepted — `docs/decisions/` exists and holds this series, and `git zhi docs
check` enforces its numbering and its exemption from the reachability check.

## Context

git-zhi has `docs/PRD/` and `docs/plans/` — eleven requirement documents and
seventeen plans. Both describe what the project intends to do. Neither records
why a particular mechanism was chosen over the alternatives that were live at
the time, so a decision's reasoning survives only in commit messages, or not
at all.

That gap has a cost the project has already paid. The postmortem was specified
as a `postmortem.md` blob on the milestone ref and shipped as a frontmatter
field instead; nothing recorded the change, so the specification and the code
disagreed silently until someone read both. The reason was good — `WriteEntity`
builds a tree with exactly one entry, so a second file would have displaced
`milestone.yaml` — but the reason lived nowhere.

A decision log is also what lets another project state a requirement without
specifying its mechanism: the requirement arrives, the decision about how to
meet it is recorded here, and the two stay distinguishable.

## Decision

Architecture decisions are recorded as ADRs in `docs/decisions/`, numbered
sequentially from 0001, in the format described by Michael Nygard in
"Documenting Architecture Decisions".

### Status

An entry is **Proposed** until the thing it decides exists in the code, and
**Accepted** once it does. Nothing else changes the status: not agreement, not
a request to build it, not time passing.

The status is a claim like any other in these documents, so it has to be one a
reader can check. "Accepted by being asked for" records a conversation nobody
can verify afterwards; "Accepted because the code is there" can be falsified by
going and looking. An entry marked Accepted with nothing built is the exact
failure this series exists to catch, wearing the series' own badge.

### Append-only, and when it starts

Append-only binds an Accepted entry. Its reasoning was live while code was
built against it, so the reasoning is history: an Accepted decision that no
longer holds is superseded by a new entry that says so and links back, never
edited into agreement with the present and never deleted.

A Proposed entry is a draft. Nothing depends on it, so it is edited in place
like any other unfinished document, and git holds the record of what it said
before. A second file explaining that the first file was wrong would duplicate
what version control already does and leave a reader to find the correction
after meeting the error.

Correcting a factual error is not editing into agreement with the present in
either case. The rule protects a record of what was believed when a decision
was made; it does not require preserving a claim that was untrue when it was
written.

`git zhi docs check` validates that the numbering has no gaps, and treats this
directory as exempt from the reachability check — ADRs are indexed by their
numbering, not by links from `CONTRIBUTING.md`.

An ADR is warranted when a choice constrains future work, when a reasonable
alternative was rejected for a reason worth remembering, or when the
implementation diverges from what a PRD or plan asserts. Routine
implementation choices do not need one.

### Relationship to other decision series

crochet keeps its own series at `docs/decisions/`, in the same shape. The two
are independent series that share a format, not one series spread across two
repositories, and that is a deliberate choice rather than an accident of
filing.

Three reasons, in descending weight. Both series number from 0001, so a bare
citation of "0002" is ambiguous the moment they are read together, and the
number is the whole of a decision's identity. A link between repositories
cannot be checked mechanically: a symmetry rule works because both halves land
in one tree in one commit, and a cross-repository link is unverifiable at
exactly the moment it matters, when the other repository moves. And crochet
already models its dependencies on git-zhi as environmental preconditions with
a version floor rather than as links, so a decision here that crochet relies on
surfaces there as a minimum version, not as a citation.

A reference to another repository's decision therefore goes in prose, naming
both — "crochet ADR 0002" — and never in a link field.

### Link fields

Relations within this series are recorded in frontmatter, so that a checker can
read them without parsing prose: `supersedes`, `superseded-by`, `amends`,
`amended-by`, each a list of four-digit numbers.

They are introduced when there is a first relation to record, not before. The
entries written so far have none, and carry none.

When they do arrive: `supersedes` and `superseded-by` are written even when
empty, because supersession is the series' core vocabulary and a reader should
meet the concept on every entry. `amends` and `amended-by` are written only
when the relation exists.

That asymmetry is not the same question as an empty `covers:` in a document's
frontmatter, which was a defect precisely because something read it — the drift
sensor consumed the list, found nothing, and reported health, so the emptiness
was load-bearing and silent. An empty `supersedes:` is inert; nothing computes
from it. The test is not whether a field may be empty. It is whether anything
depends on it being non-empty.

## Consequences

Decisions become reviewable as decisions, separately from the code that
implements them, and a superseded one remains readable alongside its successor
rather than vanishing.

The cost is discipline: an ADR written after the fact is a reconstruction, and
reconstructions are less honest than notes taken at the time. The numbering is
also a shared sequence, so two branches adding decisions concurrently will
collide on a number and one will have to renumber before merging.
