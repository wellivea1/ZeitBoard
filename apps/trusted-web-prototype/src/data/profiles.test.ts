import { describe, expect, it } from "vitest";

import { fixtureProfiles, grantedFieldNames } from "./profiles";

const metadataKeys = new Set([
  "schema_version",
  "generated_at",
  "expires_at",
  "granted_fields",
  "notice",
]);

describe("trusted fixture profiles", () => {
  it("contains only contract metadata and allowlisted projection fields", () => {
    for (const profile of Object.values(fixtureProfiles)) {
      if (!profile.projection) continue;
      for (const key of Object.keys(profile.projection)) {
        expect(metadataKeys.has(key) || grantedFieldNames.includes(key as never)).toBe(true);
      }
    }
  });

  it("includes a value only when the field is granted", () => {
    for (const profile of Object.values(fixtureProfiles)) {
      if (!profile.projection) continue;
      for (const field of grantedFieldNames) {
        expect(field in profile.projection).toBe(profile.projection.granted_fields.includes(field));
      }
    }
  });

  it("provides no projection for expired or revoked profiles", () => {
    expect(fixtureProfiles.expired.projection).toBeNull();
    expect(fixtureProfiles.revoked.projection).toBeNull();
  });
});
