export const grantedFieldNames = [
  "predicted_sleep_window",
  "predicted_waking_window",
  "confidence",
  "availability",
] as const;

export type GrantedField = (typeof grantedFieldNames)[number];
export type FixtureProfileId =
  | "planning"
  | "sleep-only"
  | "availability"
  | "default-deny"
  | "expired"
  | "revoked";

export interface MinimizedWindow {
  earliest_at: string;
  latest_at: string;
}

export interface AvailabilityWindow {
  start_at: string;
  end_at: string;
}

export interface TrustedViewProjection {
  schema_version: "v1";
  generated_at: string;
  expires_at: string;
  granted_fields: GrantedField[];
  predicted_sleep_window?: MinimizedWindow;
  predicted_waking_window?: MinimizedWindow;
  confidence?: "low" | "medium" | "high";
  availability?: AvailabilityWindow[];
  notice: "Estimated windows are uncertain and are not medical advice.";
}

export interface FixtureProfile {
  id: FixtureProfileId;
  label: string;
  description: string;
  state: "active" | "expired" | "revoked";
  projection: TrustedViewProjection | null;
}

const notice = "Estimated windows are uncertain and are not medical advice." as const;

export const fixtureProfiles: Record<FixtureProfileId, FixtureProfile> = {
  planning: {
    id: "planning",
    label: "Planning view",
    description: "Sleep, waking, confidence, and availability",
    state: "active",
    projection: {
      schema_version: "v1",
      generated_at: "2026-06-15T14:30:00Z",
      expires_at: "2026-06-22T14:30:00Z",
      granted_fields: [
        "predicted_sleep_window",
        "predicted_waking_window",
        "confidence",
        "availability",
      ],
      predicted_sleep_window: {
        earliest_at: "2026-06-15T19:10:00-04:00",
        latest_at: "2026-06-15T21:40:00-04:00",
      },
      predicted_waking_window: {
        earliest_at: "2026-06-16T03:20:00-04:00",
        latest_at: "2026-06-16T05:30:00-04:00",
      },
      confidence: "medium",
      availability: [
        {
          start_at: "2026-06-15T11:00:00-04:00",
          end_at: "2026-06-15T13:45:00-04:00",
        },
        {
          start_at: "2026-06-16T07:00:00-04:00",
          end_at: "2026-06-16T09:00:00-04:00",
        },
      ],
      notice,
    },
  },
  "sleep-only": {
    id: "sleep-only",
    label: "Sleep window only",
    description: "Predicted sleep window, with no confidence or availability",
    state: "active",
    projection: {
      schema_version: "v1",
      generated_at: "2026-06-15T14:30:00Z",
      expires_at: "2026-06-18T14:30:00Z",
      granted_fields: ["predicted_sleep_window"],
      predicted_sleep_window: {
        earliest_at: "2026-06-15T19:10:00-04:00",
        latest_at: "2026-06-15T21:40:00-04:00",
      },
      notice,
    },
  },
  availability: {
    id: "availability",
    label: "Availability only",
    description: "Coarse availability windows, with no phase estimate",
    state: "active",
    projection: {
      schema_version: "v1",
      generated_at: "2026-06-15T14:30:00Z",
      expires_at: "2026-06-17T14:30:00Z",
      granted_fields: ["availability"],
      availability: [
        {
          start_at: "2026-06-15T11:00:00-04:00",
          end_at: "2026-06-15T13:45:00-04:00",
        },
      ],
      notice,
    },
  },
  "default-deny": {
    id: "default-deny",
    label: "Default deny",
    description: "No planning fields are granted",
    state: "active",
    projection: {
      schema_version: "v1",
      generated_at: "2026-06-15T14:30:00Z",
      expires_at: "2026-06-17T14:30:00Z",
      granted_fields: [],
      notice,
    },
  },
  expired: {
    id: "expired",
    label: "Expired view",
    description: "No fields remain visible after expiry",
    state: "expired",
    projection: null,
  },
  revoked: {
    id: "revoked",
    label: "Revoked view",
    description: "No fields remain visible after revocation",
    state: "revoked",
    projection: null,
  },
};

export function getFixtureProfile(id: string | null): FixtureProfile {
  if (id && Object.hasOwn(fixtureProfiles, id)) {
    return fixtureProfiles[id as FixtureProfileId];
  }
  return fixtureProfiles.planning;
}
