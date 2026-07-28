# Synthetic test data

Everything under `testdata/` is deterministic and synthetic. It describes no
real person and must not be replaced with exported health, activity, calendar,
or account data.

Regenerate fixtures from the `tools/` module:

```sh
cd tools && go run ./cmd/genfixtures
```

CI uses `-check` to ensure checked-in fixtures match the generator. The v1
and v2 files and their validating schemas are declared in the authoritative
manifest in `tools/internal/fixtures`. Fixture tests reject missing, byte-drifted,
or unexpected stale JSON files under `testdata/`.

The v1 set includes estimated and refused phase results, private/local contract
examples, a clinical chart request, an all-false default-deny share profile, an
explicitly allowlisted profile, and their minimized trusted views.
Neither trusted view contains source identifiers, provenance, time-zone IDs,
raw observations, private calendar text, or health details.
