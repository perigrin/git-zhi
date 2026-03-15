# Issue 9: `chain list` + `chain next` + `chain config` — Design

## Scope

Three top-level commands. `chain list` uses the graph's topological sort with `--graph` and `--critical` modes. `chain next` aliases `issue show HEAD`. `chain config` reads/writes config ref.

## Commands

### `chain list`
- Default: topological sort of all issues, grouped by milestone
- `--critical`: highlight critical chain (longest sequential path)
- `--all`: include done/cancelled
- `--milestone <name>`: filter to milestone
- `--format json`

### `chain next`
- Alias for `issue show HEAD`
- Shows the current in-progress issue, or next on critical chain

### `chain config`
- Without args: display current config
- With key value: set a config option
- Supports: `default_milestone <name>`

## Files

- Modify: `internal/cli/chain.go` — replace stubs with real impls
- Create: `internal/cli/chain_list.go` + test — topological sort display
- Create: `internal/cli/chain_next.go` + test — delegates to issue show
- Create: `internal/cli/chain_config.go` + test — config read/write
- Modify: `internal/config/config.go` — add UnmarshalConfig

## Testing

- chain list: default topo sort, --critical, --all, JSON
- chain next: resolves HEAD, shows issue
- chain config: display, set default_milestone
