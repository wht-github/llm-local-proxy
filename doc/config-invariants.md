# Config Invariants

## Valid config after loading

`config.Load(path)` performs parsing and validation before returning.

If `Load` returns `cfg, nil`, then callers may assume:

- `cfg.Listen` is non-empty.
- `len(cfg.Providers) > 0`.
- Every provider has a non-empty `Name`.
- Every provider has a non-empty `Type`.
- Every provider has a non-empty `BaseURL`.

This means code after `Load` should treat `cfg` as an already validated value and should not repeat the same structural checks unless new validation rules are introduced.

## Design implication

Validation belongs at the config boundary.

Downstream code such as provider registry construction may rely on these invariants and focus on its own domain checks.
