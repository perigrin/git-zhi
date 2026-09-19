---
supersedes: []
superseded-by: []
amends: [0003]
---

# 0004. The --actor flag stays read-side

Date: 2026-09-19

## Status

Accepted — committed to by being refined into work; implemented by leaving the
write path without a flag, which is the decision.

## Context

0003's resolution order begins "an explicit `--actor` flag, where a command has
one", and never says which commands should have one. Today `next` and
`project next` do, and both are read-side. On the write path the flag tier is
therefore vacuous: `resolveActor` passes an empty explicit value because there
is nothing to pass.

That left a real question, because 0003's own consequences argue an explicit
value must be able to beat an ambient one — an exported `ZHI_ACTOR` applies to
every process in a shell and is invisible in the command that ran. If the
override matters most where state is recorded, the write path is exactly where
a flag seemed to be missing.

## Decision

No write command takes `--actor`. The flag stays on `next` and `project next`.

The override the consequences called for already exists, and it is not a flag:

    ZHI_ACTOR=human:chris git zhi issue edit <id> --state done

A variable assignment prefixed to a command applies to that process alone. It
is per-invocation by construction, it is visible in the command that ran —
which is the property the ambient case lacks — and it needs no code. Adding a
flag would be a second way to express something the shell already expresses,
and the first rule before writing anything is to check whether it needs to
exist.

Against that, two costs that are not hypothetical. `issue edit` already carries
`--assign`, which also takes a worker identity and means something different:
who *should* do the work, not who *is* doing it. Placing `--actor` beside it
invites the confusion of a lifetime of reading them in sequence. And a flag on
some write commands and not others is how a caller ends up passing it to one
and forgetting it on the next, which is the split identity 0003 exists to
prevent — the failure it names, reintroduced by the mechanism meant to serve it.

`next` and `project next` keep theirs. They are read-only, so a wrong value
costs a wrong answer rather than a wrong record, and `project next` needs the
flag for the case where an operator is asking on behalf of a worker who is not
this process.

## Consequences

0003's resolution order stands as written; this entry says which commands
satisfy its first clause. A caller who wants to record one action under a
different identity uses the environment prefix, which should be documented
where `ZHI_ACTOR` is documented rather than left to be rediscovered.

If a write command ever does need a flag, this is superseded rather than
quietly extended — the reasoning above is about a boundary, and moving it
should cost a decision.
