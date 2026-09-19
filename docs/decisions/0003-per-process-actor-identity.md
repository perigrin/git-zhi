# 0003. Actor identity is declared by the process, not derived from the commit author

Date: 2026-09-19

## Status

Proposed

## Context

`Graph.headForActor` already implements what multi-worker execution needs: an
actor holding an in-progress issue gets it back, work held by a different actor
is excluded, and an issue assigned to this actor is preferred over an
unassigned one. None of it is reachable.

Every state change records `actor.DeriveActor(Store.AuthorInfo())` — git's
`user.name` and `user.email`. Two sites do this, `issue_edit.go:300` and
`:1534`, and they are the only places a transition's actor is decided. Agents
working in one repository inherit one git author config, so `DeriveActor`
returns the same actor for all of them and the three rules collapse: every
agent "resumes" whatever any agent started, and the exclusion excludes nothing
because no other worker is ever *other*.

A worktree does not fix this. Crochet already gives each agent its own worktree,
for an unrelated reason — agents sharing an index sweep each other's files into
each other's commits — but a worktree provides a private index, not a private
identity. `user.name` resolves from repository or global config either way.

The asymmetry is the tell. The **read** side is already parameterised:
`git zhi next --actor <id>` exists, and `git zhi project next` requires
`--actor` outright. The **write** side is not. A caller can ask what is next
for `agent:worker-3` and cannot then act as `agent:worker-3`, so the answer is
unusable — and `project next` is unreachable entirely, which is why nothing has
noticed.

### The constraint

The zero-change option is a distinct `user.name` and `user.email` per agent.
It is rejected: it writes invented agents permanently into blame, `git log` and
contributor statistics. Chain identity and commit identity are different facts
about different things, and conflating them corrupts the durable one to
parameterise the ephemeral one.

So whatever carries this identity must not be the commit author.

## Decision

An actor identity is resolved once, from the first of these that is set:

1. an explicit `--actor` flag, where a command has one;
2. the `ZHI_ACTOR` environment variable;
3. `DeriveActor(AuthorInfo())` — today's behaviour.

The resolved identity is used for the actor written into a transition, and as
the default for `next --actor` and `project next --actor`, so that identity is
declared once rather than separately on each side of the same operation.
`project next` stops requiring its flag when `ZHI_ACTOR` is set.

An environment variable is the mechanism because the requirement is per
*process*, and an environment variable is the only one of these that is
per-process by construction. Git config — whether repository, global, or
worktree-scoped — is shared by every process that reads it, which is the
property that makes it wrong here and the same property that makes it right for
`zhi.sync.jira.*`. A flag alone would require every call site to pass it on
every invocation, which is how a caller ends up passing it to `next` and
forgetting it on `edit`, producing exactly the split identity this is meant to
remove.

The resolution order mirrors `jira/credentials`, which already resolves env over
git config, so this is an existing pattern rather than a new one.

`ZHI_ACTOR` must carry a recognised type prefix — `agent:` or `human:`. A value
without one is an error naming both, not a value silently recorded as human.
`ParseActor` keeps its lenient default for stored values and flags, where it is
load-bearing for backward compatibility; the strictness belongs at the point a
new identity enters the system, which is a trust boundary and the one place
guessing is expensive. An agent that states what it is beats an inference drawn
from whether its email happens to contain "bot".

Historian is explicitly out of scope. `enrich.go:192` derives actors from the
commit authors it is reconstructing, which is correct — it is recovering who
did something in the past, not declaring who is acting now, and an ambient
`ZHI_ACTOR` must not rewrite history into the current worker's name.

### Contract

Three observable behaviours, which are the acceptance criteria:

1. Two processes in one repository with identical git author config, each
   declaring a different identity, record different actors. Start an issue from
   each, read the transitions: the actors differ, and each matches what its
   process declared.
2. With nothing declared, behaviour is unchanged. A single worker on an
   existing chain records exactly what it records today. No stored chain
   changes meaning and no migration runs.
3. A declared identity parses as an actor type, and one without a recognised
   prefix is refused rather than defaulted.

## Consequences

Multi-agent execution in one repository becomes possible, and `git-zhi-project`
becomes reachable for the first time.

Commit authorship stays what it is. Blame, `git log` and contributor statistics
continue to describe who wrote the code, and the chain separately describes who
moved the work — two facts that git already keeps apart and that this keeps
apart.

The cost is an ambient input. An exported `ZHI_ACTOR` applies to every git-zhi
process in that shell, including ones the operator did not mean to attribute,
and being ambient it is invisible in the command that was run. That is the same
hazard as any environment-carried identity and the reason `--actor` sits above
it in the order: an explicit flag can always override the ambient value, and
`git zhi issue show` renders the recorded actor, so a mistake is visible after
the fact rather than silent.

A declared identity is also unverified. `ZHI_ACTOR=agent:someone-else` is
accepted exactly as given. That is appropriate for a coordination mechanism and
would not be for an authorization one; nothing here should ever become an
access control decision.

This decision is proposed. It is accepted by being asked for — an
implementation request is the acceptance, and the ADR is superseded rather than
edited if the mechanism changes after use.
