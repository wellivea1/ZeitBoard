#!/usr/bin/env python3
"""Generate deterministic, synthetic phase-one fixtures."""

from __future__ import annotations

import argparse
import json
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any


UTC = timezone.utc
GENERATED_AT = datetime(2026, 3, 15, 0, 0, tzinfo=UTC)
ZONE_ID = "America/New_York"


def timestamp(value: datetime) -> str:
    return value.astimezone(UTC).isoformat(timespec="seconds").replace("+00:00", "Z")


def uncertain_window(center: datetime, radius_minutes: int) -> dict[str, str]:
    radius = timedelta(minutes=radius_minutes)
    return {
        "earliest_at": timestamp(center - radius),
        "latest_at": timestamp(center + radius),
        "zone_id": ZONE_ID,
    }


def minimized_window(center: datetime, radius_minutes: int) -> dict[str, str]:
    radius = timedelta(minutes=radius_minutes)
    return {
        "earliest_at": timestamp(center - radius),
        "latest_at": timestamp(center + radius),
    }


def build_fixtures() -> dict[str, Any]:
    base_start = datetime(2026, 3, 5, 4, 30, tzinfo=UTC)
    cycle = timedelta(hours=24, minutes=50)
    durations = [480, 470, 490, 475, 485, 480, 465, 495, 480, 475]

    observations = []
    for index, duration_minutes in enumerate(durations):
        start = base_start + cycle * index
        end = start + timedelta(minutes=duration_minutes)
        observations.append(
            {
                "observation_id": f"obs_sleep_{index + 1:02d}",
                "kind": "sleep_episode",
                "start_at": timestamp(start),
                "end_at": timestamp(end),
                "zone_id": ZONE_ID,
                "sleep": {"classification": "principal"},
                "provenance": {
                    "acquisition_method": "synthetic",
                    "evidence_status": "directly_observed",
                    "recorded_at": timestamp(end + timedelta(minutes=5)),
                    "source_record_id": f"synthetic-sleep-{index + 1:02d}",
                },
            }
        )

    nap_start = base_start + cycle * 6 + timedelta(hours=13)
    observations.append(
        {
            "observation_id": "obs_sleep_nap_01",
            "kind": "sleep_episode",
            "start_at": timestamp(nap_start),
            "end_at": timestamp(nap_start + timedelta(minutes=45)),
            "zone_id": ZONE_ID,
            "sleep": {"classification": "nap"},
            "provenance": {
                "acquisition_method": "synthetic",
                "evidence_status": "directly_observed",
                "recorded_at": timestamp(nap_start + timedelta(minutes=50)),
                "source_record_id": "synthetic-nap-01",
            },
        }
    )

    activity_start = base_start + cycle * 8 + timedelta(hours=10)
    observations.append(
        {
            "observation_id": "obs_activity_01",
            "kind": "activity_interval",
            "start_at": timestamp(activity_start),
            "end_at": timestamp(activity_start + timedelta(minutes=30)),
            "zone_id": ZONE_ID,
            "activity": {"level": "active"},
            "provenance": {
                "acquisition_method": "synthetic",
                "evidence_status": "directly_observed",
                "recorded_at": timestamp(activity_start + timedelta(minutes=35)),
                "source_record_id": "synthetic-activity-01",
            },
        }
    )

    forecast_sleep_1 = base_start + cycle * 10
    forecast_sleep_2 = base_start + cycle * 11
    forecast_wake_1 = forecast_sleep_1 + timedelta(minutes=480)
    forecast_wake_2 = forecast_sleep_2 + timedelta(minutes=480)

    observations_fixture = {
        "schema_version": "v1",
        "generated_at": timestamp(GENERATED_AT),
        "observations": observations,
    }
    corrections_fixture = {
        "schema_version": "v1",
        "generated_at": timestamp(GENERATED_AT),
        "corrections": [
            {
                "correction_id": "cor_sleep_04_start",
                "target_observation_id": "obs_sleep_04",
                "created_at": "2026-03-08T08:25:00Z",
                "reason": "user_edit",
                "changes": {"start_at": "2026-03-08T07:10:00Z"},
            },
            {
                "correction_id": "cor_nap_excluded",
                "target_observation_id": "obs_sleep_nap_01",
                "created_at": "2026-03-12T08:00:00Z",
                "reason": "user_edit",
                "changes": {"excluded": True},
            },
        ],
    }
    estimate_fixture = {
        "schema_version": "v1",
        "status": "estimated",
        "generated_at": timestamp(GENERATED_AT),
        "algorithm_version": "theil-sen-sleep-start-v1",
        "estimate_id": "estimate_synthetic_01",
        "observed_sleep_start_drift_minutes_per_cycle": 50,
        "median_sleep_duration_minutes": 480,
        "support": {
            "observation_count": 10,
            "cycle_count": 10,
            "first_observation_at": timestamp(base_start),
            "last_observation_at": timestamp(base_start + cycle * 9),
        },
        "confidence": {
            "level": "medium",
            "reasons": [
                "ten principal synthetic sleep episodes support the estimate",
                "forecast uncertainty widens with horizon",
            ],
        },
        "forecasts": [
            {
                "cycle_index": 11,
                "predicted_sleep_window": uncertain_window(forecast_sleep_1, 30),
                "predicted_waking_window": uncertain_window(forecast_wake_1, 45),
            },
            {
                "cycle_index": 12,
                "predicted_sleep_window": uncertain_window(forecast_sleep_2, 45),
                "predicted_waking_window": uncertain_window(forecast_wake_2, 60),
            },
        ],
    }
    refused_estimate_fixture = {
        "schema_version": "v1",
        "status": "refused",
        "generated_at": timestamp(GENERATED_AT),
        "algorithm_version": "theil-sen-sleep-start-v1",
        "refusal": {
            "code": "insufficient_data",
            "message": "Need at least seven usable principal sleep episodes; found three.",
        },
    }

    schedule_request = {
        "schema_version": "v1",
        "request_id": "schedule_request_01",
        "created_at": timestamp(GENERATED_AT),
        "zone_id": ZONE_ID,
        "estimate_id": "estimate_synthetic_01",
        "tasks": [
            {
                "task_id": "task_flexible_01",
                "duration_minutes": 30,
                "earliest_at": timestamp(forecast_wake_1 + timedelta(minutes=30)),
                "latest_at": timestamp(forecast_sleep_2 - timedelta(hours=2)),
                "preference": "predicted_waking_window",
            },
            {
                "task_id": "task_flexible_02",
                "duration_minutes": 90,
                "earliest_at": timestamp(forecast_wake_1),
                "latest_at": timestamp(forecast_wake_1 + timedelta(minutes=30)),
                "preference": "any_available",
            },
        ],
        "fixed_events": [
            {
                "event_id": "event_fixed_01",
                "start_at": timestamp(forecast_wake_1 + timedelta(hours=2)),
                "end_at": timestamp(forecast_wake_1 + timedelta(hours=3)),
            }
        ],
    }
    proposal_start = forecast_wake_1 + timedelta(minutes=30)
    schedule_proposals = {
        "schema_version": "v1",
        "request_id": "schedule_request_01",
        "generated_at": timestamp(GENERATED_AT),
        "algorithm_version": "deterministic-proposal-v1",
        "proposals": [
            {
                "proposal_id": "proposal_task_01",
                "task_id": "task_flexible_01",
                "start_at": timestamp(proposal_start),
                "end_at": timestamp(proposal_start + timedelta(minutes=30)),
                "zone_id": ZONE_ID,
                "confidence": {
                    "level": "medium",
                    "reasons": ["proposal uses an uncertain predicted waking window"],
                },
                "explanation_codes": [
                    "within_predicted_waking_window",
                    "avoids_fixed_event",
                    "within_task_bounds",
                    "uncertainty_buffer_applied",
                ],
            }
        ],
        "unplaced": [
            {"task_id": "task_flexible_02", "reason": "no_available_interval"}
        ],
    }

    permissions_off = {
        "predicted_sleep_window": False,
        "predicted_waking_window": False,
        "confidence": False,
        "availability": False,
    }
    permissions_allowlisted = {
        "predicted_sleep_window": True,
        "predicted_waking_window": True,
        "confidence": True,
        "availability": False,
    }
    share_profile_default_deny = {
        "schema_version": "v1",
        "profile_id": "share_profile_deny_01",
        "state": "active",
        "created_at": timestamp(GENERATED_AT),
        "expires_at": timestamp(GENERATED_AT + timedelta(hours=24)),
        "permissions": permissions_off,
    }
    share_profile_allowlisted = {
        "schema_version": "v1",
        "profile_id": "share_profile_allow_01",
        "state": "active",
        "created_at": timestamp(GENERATED_AT),
        "expires_at": timestamp(GENERATED_AT + timedelta(hours=24)),
        "permissions": permissions_allowlisted,
    }
    trusted_view_default_deny = {
        "schema_version": "v1",
        "generated_at": timestamp(GENERATED_AT),
        "expires_at": timestamp(GENERATED_AT + timedelta(hours=24)),
        "granted_fields": [],
        "notice": "Estimated windows are uncertain and are not medical advice.",
    }
    trusted_view = {
        "schema_version": "v1",
        "generated_at": timestamp(GENERATED_AT),
        "expires_at": timestamp(GENERATED_AT + timedelta(hours=24)),
        "granted_fields": [
            "predicted_sleep_window",
            "predicted_waking_window",
            "confidence",
        ],
        "predicted_sleep_window": minimized_window(forecast_sleep_1, 30),
        "predicted_waking_window": minimized_window(forecast_wake_1, 45),
        "confidence": "medium",
        "notice": "Estimated windows are uncertain and are not medical advice.",
    }

    return {
        "observations.json": observations_fixture,
        "corrections.json": corrections_fixture,
        "phase-estimate.json": estimate_fixture,
        "phase-estimate-refused.json": refused_estimate_fixture,
        "schedule-request.json": schedule_request,
        "schedule-proposals.json": schedule_proposals,
        "share-profile-default-deny.json": share_profile_default_deny,
        "share-profile-allowlisted.json": share_profile_allowlisted,
        "trusted-view-default-deny.json": trusted_view_default_deny,
        "trusted-view.json": trusted_view,
    }


def encoded(value: Any) -> str:
    return json.dumps(value, indent=2, sort_keys=False, ensure_ascii=True) + "\n"


def assert_fixture_safety(fixtures: dict[str, Any]) -> None:
    observations = fixtures["observations.json"]["observations"]
    if any(item["provenance"]["acquisition_method"] != "synthetic" for item in observations):
        raise ValueError("all generated observations must use synthetic provenance")

    forbidden_keys = {
        "calendar_text",
        "diagnosis",
        "location",
        "medication",
        "notes",
        "observation_id",
        "profile_id",
        "provenance",
        "raw_activity",
        "zone_id",
    }

    def walk(value: Any) -> None:
        if isinstance(value, dict):
            overlap = forbidden_keys.intersection(value)
            if overlap:
                raise ValueError(f"trusted view contains forbidden keys: {sorted(overlap)}")
            for child in value.values():
                walk(child)
        elif isinstance(value, list):
            for child in value:
                walk(child)

    walk(fixtures["trusted-view-default-deny.json"])
    walk(fixtures["trusted-view.json"])


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path(__file__).resolve().parents[1] / "testdata" / "v1",
        help="fixture output directory",
    )
    parser.add_argument(
        "--check",
        action="store_true",
        help="fail when checked-in fixtures differ from deterministic output",
    )
    args = parser.parse_args()

    fixtures = build_fixtures()
    assert_fixture_safety(fixtures)
    mismatches = []

    for name, value in fixtures.items():
        path = args.output_dir / name
        content = encoded(value)
        if args.check:
            if not path.exists() or path.read_text(encoding="utf-8") != content:
                mismatches.append(str(path))
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding="utf-8", newline="\n")

    if mismatches:
        print("fixture drift detected:", file=sys.stderr)
        for path in mismatches:
            print(f"  {path}", file=sys.stderr)
        print("run scripts/generate-testdata.py", file=sys.stderr)
        return 1

    action = "verified" if args.check else "generated"
    print(f"{action} {len(fixtures)} synthetic fixture files in {args.output_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
