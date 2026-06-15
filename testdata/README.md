# Synthetic test data

Everything under `testdata/` is deterministic and synthetic. It describes no
real person and must not be replaced with exported health, activity, calendar,
or account data.

Regenerate fixtures from the repository root:

```sh
python scripts/generate-testdata.py
```

CI uses `--check` to ensure checked-in fixtures match the generator. The v1
set includes estimated and refused phase results, private/local contract
examples, an all-false default-deny share profile, an explicitly allowlisted
profile, and their minimized trusted views.
Neither trusted view contains source identifiers, provenance, time-zone IDs,
raw observations, private calendar text, or health details.
