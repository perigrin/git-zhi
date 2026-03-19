## title: Perl Development Plugin PRD
description: "Product requirements document for the perl-development plugin — agentic Perl development skills for the claude-plugins-marketplace"

# perl-development Plugin — Product Requirements Document

## Overview

The `perl-development` plugin provides a complete agentic Perl development
skill library for Claude Code. It makes Perl projects first-class citizens in
the Claude Code ecosystem — with version-aware code generation, idiomatic
testing, dependency management, static analysis, and regression testing across
the production version matrix.

The plugin defines a **Perl development interface**. PVM is the reference
toolchain implementation. Alternative backends (plenv, perlbrew, or custom
setups) are a community contribution path — the plugin is architected to
support them, but only PVM ships in v0.1.

## Problem Statement

Perl is a first-class agentic development language. Its syntax is expressive,
its ecosystem is deep, and its modern OO system (`feature 'class'`) is
well-suited to agent-generated code. But Claude Code has no native Perl skill
library. Agents working in Perl projects today must:

- Guess which Perl version is active, often defaulting to outdated idioms
- Reach for `@_` extraction instead of signatures
- Write `Test::More` tests instead of `Test2::V0`
- Manage CPAN dependencies manually without knowing what toolchain is present
- Test against a single Perl version when the project targets a production
  matrix spanning 5.26 through 5.42

## Design Principles

1. **Capability over toolchain.** The plugin defines what Perl development
   needs (a version manager, a module installer, a script runner, a source
   analyzer) and maps those capabilities to concrete commands via backend
   skills. PVM is the default backend; others are possible.
1. **Ask, don't guess.** When setting up a project, the plugin asks which
   toolchain to use rather than detecting and assuming. If a backend is clearly
   already in use, it confirms. Default is PVM.
1. **Version awareness is automatic.** The active Perl version is detected
   from `.perl-version`, `use VERSION` statements, or the configured backend.
1. **Skills are self-announcing.** Each skill announces itself at invocation.
1. **Modern Perl by default.** When version detection is ambiguous, defaults
   to the highest installed version.
1. **Test2 everywhere.** All testing skills use `Test2::V0`. `Test::More` is
   a migration target.
1. **`Feature::Compat::Class` for pre-5.38 code only.** Native `feature 'class'`
   is available from 5.38 onwards. Code targeting < 5.38 uses
   `Feature::Compat::Class` as the bridge.
1. **Test against the real version matrix.** PVM's binary cache makes
   regression testing against 7+ Perl versions practical.
1. **Real-data tests, not toy examples.** Skills warn explicitly against
   test theater.

-----

## Toolchain Backend Skills

The backend skill is the key abstraction that keeps the rest of the plugin
toolchain-agnostic. Each backend skill contains two things: how to **install**
the toolchain, and how to **use** it once installed.

### `perl:using-pvm` (reference implementation)

`perl:using-pvm` is self-contained. It knows how to install PVM for the
current platform and how to use all four PVM components once installed. All
other skills that need to run Perl include this skill's capability map.

#### Installation

**Step 1: Detect platform**

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')   # linux or darwin
ARCH=$(uname -m)                               # x86_64 or arm64/aarch64
```

Normalize architecture:

```bash
case "$ARCH" in
  x86_64)           ARCH="amd64" ;;
  arm64|aarch64)    ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1 ;;
esac
```

Supported platforms:

|`uname -s`|`uname -m`         |Binary name       |
|----------|-------------------|------------------|
|`linux`   |`x86_64`           |`pvm-linux-amd64` |
|`linux`   |`arm64` / `aarch64`|`pvm-linux-arm64` |
|`darwin`  |`x86_64`           |`pvm-darwin-amd64`|
|`darwin`  |`arm64`            |`pvm-darwin-arm64`|

Windows (PowerShell) is out of scope for v0.1.

**Step 2: Confirm with user**

> "PVM is not installed. I'll download `pvm-${OS}-${ARCH}` from GitHub
> releases and place it in `~/.local/bin/`. Install now?"

Wait for confirmation before proceeding.

**Step 3: Download and install**

```bash
INSTALL_DIR="$HOME/.local/bin"
mkdir -p "$INSTALL_DIR"

# Resolve the latest release tag (including pre-releases) via GitHub API.
VERSION=$(curl -fsSL "https://api.github.com/repos/perigrin/pvm/releases" \
  | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/')
VERSION_NUM=${VERSION#v}

ASSET="pvm-${VERSION_NUM}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/perigrin/pvm/releases/download/${VERSION}/${ASSET}"

curl -fsSL "$URL" -o "/tmp/${ASSET}"
tar -xzf "/tmp/${ASSET}" -C /tmp
# The tarball contains a binary named pvm-<version>-<os>-<arch>.
BINARY=$(ls /tmp/pvm-*-${OS}-${ARCH} 2>/dev/null | head -1)
chmod +x "$BINARY"
mv "$BINARY" "${INSTALL_DIR}/pvm"

rm -f "/tmp/${ASSET}"
```

**Step 4: PATH check**

```bash
if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
    echo "⚠ $INSTALL_DIR is not in your PATH."
    echo "Add this to your shell profile (~/.bashrc, ~/.zshrc, etc.):"
    echo '  export PATH="$HOME/.local/bin:$PATH"'
    echo "Then reload your shell or run: export PATH=\"\$HOME/.local/bin:\$PATH\""
fi
```

**Step 5: Create symlinks**

```bash
pvm self symlinks create
```

This creates `pvx`, `pvi`, and `psc` alongside the `pvm` binary.

**Step 6: Verify**

```bash
pvm version
pvx --version
psc --version
```

All four must succeed. If any fail, report the specific error — do not
silently continue.

#### Capability map

Once installed, use these commands:

|Capability           |Command                                         |
|---------------------|------------------------------------------------|
|Install Perl version |`pvm install <version>`                         |
|Switch Perl version  |`PVM_PERL_VERSION=<ver>` or `.perl-version` file|
|Current version      |`pvm current`                                   |
|Installed versions   |`pvm versions`                                  |
|Install CPAN module  |`pvm module install <Module>`                   |
|Install from cpanfile|`pvm module install`                            |
|Run script isolated  |`pvx <script>`                                  |
|Run tests            |`pvx prove -lr t/`                              |
|Parse source / AST   |`psc parse <file>`                              |
|Analyze dependencies |`psc analyze lib/`                              |
|Regression matrix    |`PVM_PERL_VERSION=<ver> pvx prove -lr t/`       |

-----

### `perl:using-plenv` (community contribution — not in v0.1)

Would provide the same sections — install procedure for plenv + cpanm, and
a capability table with `plenv exec perl`, `cpanm`, `perl -Ilib prove`
substituted in.

-----

### `perl:using-perlbrew` (community contribution — not in v0.1)

Same pattern with `perlbrew exec`, `cpanm`, `perlbrew exec prove`.

-----

### `perl:using-custom` (v0.1)

For bespoke or corporate Perl setups. During `perl:setup`, if the user
selects "Other", they are prompted to fill in the capability table manually.
The result is written to the project's CLAUDE.md.

```markdown
## Toolchain: Custom

Fill in commands for your Perl setup:

| Capability | Command |
|---|---|
| Install Perl version | ___________________ |
| Switch Perl version | ___________________ |
| Current version | ___________________ |
| Install CPAN module | ___________________ |
| Run script | ___________________ |
| Run tests | ___________________ |
| Parse source | ___________________ |
```

-----

## Skills

### `perl:setup`

**Purpose:** Scaffold a Perl project for agentic development, configuring
the appropriate toolchain backend.

**Trigger:** User says "set up Perl", "initialize this Perl project", or any
skill fails because no toolchain is configured.

**Process:**

**Phase 1: Toolchain selection**

Check for evidence of an existing toolchain:

- Is `pvm` on `$PATH`?
- Is `plenv` on `$PATH`?
- Is `perlbrew` on `$PATH`?
- Is there a `.perl-version` file?

If a backend is clearly in use, confirm:

> "I can see PVM is installed. Should I configure this project to use PVM?
> (Other options: plenv, perlbrew, custom)"

If nothing detected or ambiguous, ask:

> "Which Perl toolchain would you like to use?
>
> 1. PVM (default, recommended)
> 1. plenv
> 1. perlbrew
> 1. Other (configure manually)"

**Phase 2: Backend setup**

Delegate entirely to the selected backend skill:

- **PVM selected:** invoke `perl:using-pvm` — it handles installation if
  needed, PATH checking, symlink creation, and verification
- **plenv selected:** invoke `perl:using-plenv` (not in v0.1 — explain and
  offer PVM or custom instead)
- **perlbrew selected:** invoke `perl:using-perlbrew` (not in v0.1 — same)
- **Other:** invoke `perl:using-custom` — prompt user to fill in capability table

**Phase 3: Perl version setup**

Check for `.perl-version`. If absent:

1. Scan for `use VERSION` in existing `.pm` or `.pl` files
1. If none found, ask which Perl version to target
1. Install if needed (`perl:using-pvm` handles this via `pvm install`)
1. Write `.perl-version`

**Phase 4: Dependency file setup (if needed)**

If no `cpanfile` exists:

```
# cpanfile
requires 'perl', '5.036';

on test => sub {
    requires 'Test2::V0';
};
```

Adjust `perl` version to match `.perl-version`.

**Phase 5: CLAUDE.md injection**

Append the Perl development block if absent:

```markdown
## Perl Development

<!-- Backend: pvm -->
<!-- Include the perl:using-pvm skill for toolchain commands -->

### Version Detection

Before writing Perl code, detect the active version:

1. Read `.perl-version` if present
2. Run `pvm current` (or backend equivalent) if absent
3. Check for `use VERSION` in existing source files

Select the appropriate writing skill:
- Perl 5.42.x → `perl:write-5.42`
- Perl 5.40.x → `perl:write-5.40`
- Perl 5.38.x → `perl:write-5.38`
- Perl 5.36.x → `perl:write-5.36`
- Perl 5.20–5.34 or CPAN dist → `perl:write-toolchain`
```

The `<!-- Backend: pvm -->` comment is machine-readable — `perl:require-toolchain`
reads it to determine which capability map to include.

**Phase 6: Migrate from superpowers writing skills**

Check for legacy `writing-perl-*` skills in `~/.claude/skills/`:

```bash
ls ~/.claude/skills/writing-perl-* 2>/dev/null
```

If found, offer to remove them:

> "I found legacy Perl writing skills in ~/.claude/skills/:
>   writing-perl-5.42.0, writing-perl-5.38.0, writing-perl-toolchain
>
> The perl-development plugin supersedes these with corrected versions.
> Remove the old skills? (They can be reinstalled from superpowers if needed.)"

On confirmation, remove the old skill directories. This prevents Claude Code
from having two competing skills for the same task.

-----

### `perl:require-toolchain` (shared dependency)

**Purpose:** Verify that a Perl toolchain is configured. Included by all
skills that need to run Perl. Not invoked directly by users.

**Process:**

1. Check CLAUDE.md for a `<!-- Backend: ... -->` comment
1. If found, include the corresponding `perl:using-*` skill
1. If not found:

> "No Perl toolchain is configured for this project.
> Run `perl:setup` to configure one."
> Do not proceed.

-----

### `perl:detect-version` (shared dependency)

**Purpose:** Detect the active Perl version. Not invoked directly.

**Process:**

1. Read `.perl-version` in current or ancestor directories
1. If absent, run the backend's "current version" command
1. If no backend, scan for `use VERSION` in `lib/**/*.pm`
1. Return: version string, detection source, recommended skill name

**Version → skill mapping:**

|Version range      |Skill                                         |
|-------------------|----------------------------------------------|
|5.42.x             |`perl:write-5.42`                             |
|5.40.x             |`perl:write-5.40`                             |
|5.38.x             |`perl:write-5.38`                             |
|5.36.x             |`perl:write-5.36`                             |
|< 5.36 or CPAN dist|`perl:write-toolchain`                        |
|Unknown            |Ask; default to `perl:write-5.42` if confirmed|

-----

### `perl:write-5.42`

**Purpose:** Write modern Perl 5.42 code.

**Announce:** "I'm using the perl:write-5.42 skill to write modern Perl code."

**Prerequisites:** `perl:require-toolchain`

**Source:** Migrated from `writing-perl-5.42.0` with corrections:

- `Feature::Compat::Class` not used — native `feature 'class'` is correct
- Operator overloading carries real runtime cost — prefer explicit methods
  in performance-sensitive code
- `no warnings 'experimental::class'` required until feature stabilizes
- All files: `use 5.42.0; use utf8;` — 5.42 defaults to ASCII source encoding

-----

### `perl:write-5.40`

**Purpose:** Write Perl 5.40 code.

**Announce:** "I'm using the perl:write-5.40 skill to write Perl 5.40 code."

**Key differences from 5.42:**

- No `:writer` field attribute (added 5.42)
- No lexical methods `my method` (added 5.42)
- No `source::encoding 'ascii'` default
- `feature 'class'` experimental — `no warnings 'experimental::class'`
- `:5.40` builtin bundle auto-exported
- `Feature::Compat::Class` not needed

**Standard boilerplate:**

```perl
use 5.040;
no warnings 'experimental::class';
```

**Note:** 5.40 is the minimum reliable version for `feature 'class'` —
5.38 had segfault bugs with same-file parent/subclass and refaliasing with
field variables, both fixed in 5.40.

-----

### `perl:write-5.38`

**Purpose:** Write Perl 5.38 code.

**Announce:** "I'm using the perl:write-5.38 skill to write Perl 5.38 code."

**Key differences from 5.40:**

- No auto-exported builtins — explicit import required:
  `use builtin qw(true false blessed refaddr trim);`
- `feature 'class'` has known segfault bugs in 5.38 — same-file
  parent/subclass and refaliasing with field variables can segfault.
  Use `Feature::Compat::Class` instead of native `feature 'class'` on 5.38.
  5.40 is the minimum reliable version for native class support.

**Standard boilerplate:**

```perl
use 5.038;
use strict;
use warnings;
use builtin qw(true false blessed refaddr);
```

**If class syntax needed (use Feature::Compat::Class, not native):**

```perl
use Feature::Compat::Class 0.07;  # 0.07 for :reader
```

-----

### `perl:write-5.36`

**Purpose:** Write Perl 5.36 code.

**Announce:** "I'm using the perl:write-5.36 skill to write Perl 5.36 code."

**Key differences from 5.38:**

- No `feature 'class'` — use `Moo`, `Moose`, `Object::Pad`, or blessed refs
- `Feature::Compat::Class` available if class syntax needed (5.14+ via
  Object::Pad polyfill, core-syntax subset only)
- No builtins without explicit import
- `use warnings` auto-enabled by `use 5.036`
- Signatures stable — use them

**Standard boilerplate:**

```perl
use 5.036;
```

**If class syntax needed:**

```perl
use 5.036;
use Feature::Compat::Class 0.07;  # 0.07 for :reader; 0.08 for :writer
```

-----

### `perl:write-toolchain`

**Purpose:** Write broadly compatible Perl for CPAN distributions and
toolchain modules targeting Perl 5.20+.

**Announce:** "I'm using the perl:write-toolchain skill to write broadly compatible Perl code."

**Source:** Migrated from `writing-perl-toolchain` with additions:

- `Feature::Compat::Class` for < 5.38 targets only — core-syntax subset
- `Test2::V0` preferred even for toolchain code (supports 5.8.1+)
- Minimize non-core dependencies (`corelist <Module>` before adding)

-----

### `perl:test`

**Purpose:** Write and run Perl tests using Test2::V0.

**Announce:** "I'm using the perl:test skill to write Perl tests."

**Prerequisites:** `perl:require-toolchain`

**Standard boilerplate:**

```perl
use Test2::V0;

# ... tests

done_testing;
```

**Key Test2::V0 patterns:**

- `is()` does deep comparison — replaces `is` and `is_deeply`
- `like()` for regex / structural matching
- `ok()` for boolean assertions
- `dies_ok { }`, `lives_ok { }` (replaces Test::Exception)
- `warnings_like { }` (replaces Test::Warn)
- `mock()` for scoped auto-cleaning mocking
- `subtest 'name' => sub { }` for grouped tests
- `done_testing` — no plan required

**Running tests (PVM backend):**

```bash
pvx prove -lr t/
pvx prove -lv t/unit/x.t
pvx yath t/
```

**Test theater warning:** Tests must use **real inputs**, not toy examples.
A parser should parse actual files. A serializer should round-trip real data.
100% coverage with synthetic inputs is false confidence — real inputs expose
bugs that synthetic ones miss. Always validate against production-representative
data before considering a test suite complete.

**TDD workflow:**

1. Write failing test first
1. Write minimal implementation
1. Run — expect pass
1. Refactor
1. Run full suite

**Parallel review pattern:** After each significant implementation phase,
run three parallel review agents:

1. **Correctness reviewer** — does it match requirements?
1. **Test coverage reviewer** — are tests comprehensive and using real data?
1. **Performance reviewer** — scaling issues, unnecessary allocations?

**Migration from Test::More:**

- `is_deeply($a, $b)` → `is($a, $b)`
- `isa_ok($obj, 'Class')` → `isa_ok($obj, ['Class'], 'message')`
- Remove `plan tests => N`; keep `done_testing`

-----

### `perl:test-mojolicious`

**Purpose:** Test Mojolicious web applications.

**Announce:** "I'm using the perl:test-mojolicious skill for Mojolicious testing."

**Prerequisites:** `perl:require-toolchain`

**Testing strategy:**

|Test type           |Tool                                |
|--------------------|------------------------------------|
|HTTP / JSON / API   |`Test::Mojo`                        |
|DOM + HTMX          |`Test::Mojo` CSS selector assertions|
|JavaScript execution|`agent-browser` or Playwright CLI   |
|Visual / layout     |Playwright CLI screenshot           |

`Test::Mojo` is the Mojolicious-maintained testing tool that ships with the
framework. Prefer it over third-party alternatives like `Test2::MojoX` (last
updated 2021) unless the project already uses it.

```perl
use Test::More;
use Test::Mojo;

my $t = Test::Mojo->new('MyApp');
$t->get_ok('/')->status_is(200)->content_like(qr/Welcome/);
$t->get_ok('/api/items')->status_is(200)->json_is('/0/name' => 'Widget');
$t->post_ok('/login' => form => {user => 'alice', pass => 'secret'})
  ->status_is(302)->header_is(Location => '/dashboard');

done_testing;
```

Note: `Test::Mojo` uses `Test::More` internally. This is the one exception to
the "Test2 everywhere" principle — Mojolicious's test infrastructure is tightly
coupled to `Test::More` and works correctly as-is.

-----

### `perl:debug`

**Purpose:** Systematic debugging using the configured toolchain.

**Announce:** "I'm using the perl:debug skill for systematic Perl debugging."

**Prerequisites:** `perl:require-toolchain`

**Tool hierarchy (PVM backend):**

1. `psc parse <file>` — AST inspection
1. `psc analyze lib/` — dependency analysis
1. `pvx -w <script>` — run with warnings
1. `pvx perl -d:Trace <script>` — execution tracing
1. `pvx perl -MDevel::Cover <script>` — coverage

**Process:**

1. Reproduce: run failing test in isolation (verbose)
1. Parse affected files with `psc parse`
1. Add targeted inspection: `use Data::Dumper; say STDERR Dumper($suspect)`
1. Run with warnings
1. Lint at severity 1
1. Remove all debug output before committing

-----

### `perl:manage-deps`

**Purpose:** Install and manage CPAN dependencies.

**Announce:** "I'm using the perl:manage-deps skill to manage Perl dependencies."

**Prerequisites:** `perl:require-toolchain`

**PVM backend:**

```bash
pvm module install Mojolicious
pvm module install            # from cpanfile
pvm module list | grep Module
```

**cpanfile format:**

```
requires 'Mojolicious', '9.0';
recommends 'Cpanel::JSON::XS';

on test => sub {
    requires 'Test2::V0';
    requires 'Test2::Suite';
};

on develop => sub {
    requires 'Perl::Critic';
    requires 'Perl::Tidy';
    requires 'Devel::Cover';
};
```

**`Feature::Compat::Class` — only for < 5.38 targets:**

- `requires 'Feature::Compat::Class', '0.07'` — for `:reader`
- `requires 'Feature::Compat::Class', '0.08'` — for `:writer`

-----

### `perl:review`

**Purpose:** Static analysis and code review.

**Announce:** "I'm using the perl:review skill for Perl static analysis."

**Prerequisites:** `perl:require-toolchain`

```bash
perltidy -b lib/**/*.pm t/**/*.t
perlcritic --severity 3 lib/
perlcritic --severity 1 lib/        # pre-release
```

**Standard `.perlcriticrc`:**

```ini
severity = 3
theme = core

[-Perl::Critic::Policy::Documentation::RequirePodAtEnd]
[-Perl::Critic::Policy::Documentation::RequirePodSections]
[-Perl::Critic::Policy::Modules::RequireVersionVar]
[-Perl::Critic::Policy::ClassHierarchies::ProhibitExplicitISA]
```

**What to flag:**

- `@_` extraction when `method` keyword is available
- `Test::More` in tests — migrate to `Test2::V0`
- `print` instead of `say`
- `1/0` booleans when `true/false` available (5.40+)
- Missing `use utf8` in 5.42 files with non-ASCII
- `Feature::Compat::Class` in 5.38+ code (use native `feature 'class'`)
- Operator overloading in performance-sensitive paths

-----

### `perl:regression-test`

**Purpose:** Run tests against a matrix of Perl versions using PVM's
binary cache.

**Announce:** "I'm using the perl:regression-test skill to test across Perl versions."

**Prerequisites:** `perl:require-toolchain`

**Note:** This skill requires PVM. The binary cache — pre-compiled Perl
binaries downloadable in seconds — is what makes a 7-version matrix practical.
Perlbrew and plenv require compiling from source (30–60 minutes per version).
Community contributions for non-PVM regression testing are welcome; the
architecture supports them.

**Named matrices:**

|Matrix         |Versions         |Justification                                    |
|---------------|-----------------|-------------------------------------------------|
|`pvm-legacy`   |5.26, 5.32       |RHEL 8 (until 2029), RHEL 9 / AL2023 (until 2032)|
|`pvm-stable`   |5.34, 5.36, 5.38 |Ubuntu 22.04, Debian 12, Ubuntu 24.04            |
|`pvm-current`  |5.40, 5.42       |RHEL 10, Debian 13, Docker, Lambda               |
|`pvm-modern`   |5.32–5.42        |Broad compatibility excluding RHEL 8             |
|`pvm-full`     |5.26–5.42 (all 7)|Maximum production coverage                      |
|`pvm-toolchain`|all 7            |CPAN distribution releases                       |

**Version justifications:**

|Version|Platform                                 |Supported until       |
|-------|-----------------------------------------|----------------------|
|5.26   |RHEL 8 (AlmaLinux / Rocky)               |2029 full / 2032 ELS  |
|5.32   |RHEL 9, Amazon Linux 2023, Debian 11     |2032 / 2029 / Aug 2026|
|5.34   |Ubuntu 22.04, macOS system Perl          |2027 std / 2032 ESM   |
|5.36   |Debian 12 Bookworm                       |June 2028             |
|5.38   |Ubuntu 24.04, Alpine 3.20, WSL default   |2029 std / 2036 ESM   |
|5.40   |RHEL 10, Debian 13, Docker maintained    |~2035                 |
|5.42   |Docker latest, Alpine 3.23, Lambda layers|Upstream current      |

5.28 and 5.30 excluded — no major distribution ships them in active support.
Watch for **Perl 5.44** (~July 2026): update `pvm-current` to `[5.42, 5.44]`
and move 5.40 to `pvm-stable`.

**Workflow:**

```bash
# Install matrix (minutes via binary cache)
# Patch versions current as of March 2026 — update periodically.
pvm install 5.26.3 5.32.1 5.34.4 5.36.3 5.38.4 5.40.2 5.42.0

# Run
for version in 5.26.3 5.32.1 5.34.4 5.36.3 5.38.4 5.40.2 5.42.0; do
    result=$(PVM_PERL_VERSION=$version pvx prove -lr t/ 2>&1 | tail -1)
    echo "$version: $result"
done
```

**Failure triage:**

1. Isolate: `PVM_PERL_VERSION=5.32.1 pvx prove -lv t/failing.t`
1. Parse: `psc parse lib/Affected.pm`
1. Classify:
- **Syntax incompatibility** — feature unavailable in that version
- **Missing module** — not in core (`corelist <Module>`)
- **Logic failure** — actual bug, version-independent

**Matrix config in `.pvm/pvm.toml`:**

```toml
[regression]
default_matrix = "pvm-modern"

[regression.custom]
versions = ["5.32.1", "5.38.2", "5.42.0"]
```

-----

## Commands

```
commands/
  setup.md              → perl:setup
  write-perl.md         → perl:detect-version + appropriate write skill
  test-perl.md          → perl:test
  test-mojolicious.md   → perl:test-mojolicious
  debug-perl.md         → perl:debug
  manage-deps.md        → perl:manage-deps
  review-perl.md        → perl:review
  regression-test.md    → perl:regression-test
```

`write-perl.md` is the primary code generation entry point — version detection
and dispatch happen automatically.

-----

## Plugin Metadata (`plugin.json`)

```json
{
  "name": "perl-development",
  "description": "Agentic Perl development — version-aware code generation, Test2 testing, dependency management, static analysis, and regression testing across the production version matrix. PVM is the reference backend; alternative backends are a community contribution path.",
  "version": "0.1.0",
  "author": {
    "name": "Chris Prather"
  },
  "homepage": "https://github.com/perigrin/perl-development-plugin",
  "repository": "https://github.com/perigrin/perl-development-plugin",
  "license": "Artistic-2.0",
  "skills": "./skills/",
  "commands": "./commands/"
}
```

-----

## File Structure

```
perl-development-plugin/
  .claude-plugin/
    plugin.json
  skills/
    setup/
      setup.md
    detect-version/
      detect-version.md
    require-toolchain/
      require-toolchain.md
    using-pvm/
      using-pvm.md              ← install procedure + capability map
    writing-perl-5.42/
      writing-perl-5.42.md
    writing-perl-5.40/
      writing-perl-5.40.md
    writing-perl-5.38/
      writing-perl-5.38.md
    writing-perl-5.36/
      writing-perl-5.36.md
    writing-perl-toolchain/
      writing-perl-toolchain.md
    testing-perl/
      testing-perl.md
    testing-mojolicious/
      testing-mojolicious.md
    debugging-perl/
      debugging-perl.md
    managing-perl-deps/
      managing-perl-deps.md
    reviewing-perl/
      reviewing-perl.md
    regression-testing/
      regression-testing.md
  commands/
    setup.md
    write-perl.md
    test-perl.md
    test-mojolicious.md
    debug-perl.md
    manage-deps.md
    review-perl.md
    regression-test.md
  README.md
```

-----

## Skill Summary Table

|Skill                   |Command                       |Purpose                            |
|------------------------|------------------------------|-----------------------------------|
|`perl:setup`            |`perl:setup`                  |Select backend, scaffold project   |
|`perl:using-pvm`        |(injected by setup)           |PVM install + capability map       |
|`perl:require-toolchain`|(internal)                    |Verify backend configured          |
|`perl:detect-version`   |(internal)                    |Detect active Perl version         |
|`perl:write-5.42`       |`perl:write-perl` (dispatched)|Modern Perl 5.42                   |
|`perl:write-5.40`       |`perl:write-perl` (dispatched)|Perl 5.40                          |
|`perl:write-5.38`       |`perl:write-perl` (dispatched)|Perl 5.38                          |
|`perl:write-5.36`       |`perl:write-perl` (dispatched)|Perl 5.36                          |
|`perl:write-toolchain`  |`perl:write-perl` (dispatched)|CPAN / toolchain 5.20+             |
|`perl:test`             |`perl:test-perl`              |Test2::V0, prove, real-data testing|
|`perl:test-mojolicious` |`perl:test-mojolicious`       |Test::Mojo                         |
|`perl:debug`            |`perl:debug-perl`             |Source analysis, debugging         |
|`perl:manage-deps`      |`perl:manage-deps`            |Module install, cpanfile           |
|`perl:review`           |`perl:review-perl`            |perlcritic, perltidy               |
|`perl:regression-test`  |`perl:regression-test`        |Matrix testing via binary cache    |

-----

## Non-Goals (v0.1)

- **plenv / perlbrew backend skills** — architecture supports them; PVM ships first
- **Moose/Moo skills** — native class syntax and blessed refs only
- **Dancer2 / Catalyst testing skills** — Mojolicious only
- **PSC type inference skills** — deferred until PSC's type inference stabilizes
- **Game development skills** — Raylib::FFI etc. deferred
- **Windows PowerShell** — Unix shell assumed for v0.1
- **Raku** — separate toolchain

-----

## Community Contribution Path

To add a new toolchain backend (e.g. plenv):

1. Create `skills/using-plenv/using-plenv.md` with:
- Install procedure for plenv + cpanm
- Capability table with concrete commands
1. Update `perl:setup` to offer plenv as an option
1. Update `perl:require-toolchain` to recognize `<!-- Backend: plenv -->`
1. Note any limitations (e.g. no binary cache means regression testing is
   slower — document this clearly rather than hiding it)

The writing, testing, and review skills require no changes — they work through
the capability abstraction.

-----

## Future Considerations

1. **`perl:write-with-types`** — Once PSC's type inference stabilizes. The
   Chalk project (perigrin/chalk, bootstrap branch) is the reference consumer.
1. **`perl:migrate-perl`** — Systematic modernization: `Test::More` →
   `Test2::V0`, blessed refs → `feature 'class'`, `@_` → signatures.
1. **`perl:cpan-dist`** — CPAN distribution authoring: Dist::Zilla / Minilla,
   PAUSE upload, changelog management.
1. **`perl:mojolicious`** — Deeper Mojolicious skills beyond testing.
1. **`perl:raylib`** — Raylib::FFI and game development patterns.
1. **Perl 5.44 matrix update** — ~July 2026: update `pvm-current` to
   `[5.42, 5.44]`; move 5.40 to `pvm-stable`; 5.38 exits upstream security.

-----

## Marketplace Entry

```json
{
  "name": "perl-development",
  "source": {
    "source": "url",
    "url": "https://github.com/perigrin/perl-development-plugin.git"
  },
  "description": "Agentic Perl development — version-aware code generation, Test2 testing, dependency management, static analysis, and regression testing. PVM is the reference backend.",
  "version": "0.1.0"
}
```

Installation:

```
/install-plugin perl-development@perigrin-marketplace
```

-----

## Relationship to Other Plugins

|Plugin           |Relationship                                                                                                                                |
|-----------------|--------------------------------------------------------------------------------------------------------------------------------------------|
|`crochet`        |`perl-development` enables Perl projects to use crochet. A project with both gets full zhi+crochet workflow with Perl-aware code generation.|
|`commonplacebook`|Independent.                                                                                                                                |

The Chalk project (perigrin/chalk, bootstrap branch) is the reference consumer
— a self-hosting Earley parser approaching XS compilation. Its CLAUDE.md
independently developed the parallel review pattern and real-data testing
requirement that this plugin formalizes.
