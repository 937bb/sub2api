import { describe, expect, it } from "vitest";

import { normalizeQuotaBypassScheduling } from "../groupsQuotaBypass";

describe("normalizeQuotaBypassScheduling", () => {
  it("clears concentrated scheduling when quota bypass is disabled", () => {
    const form = {
      quota_bypass_enabled: false,
      quota_bypass_concentrated_scheduling_enabled: true,
    };

    normalizeQuotaBypassScheduling(form);

    expect(form.quota_bypass_concentrated_scheduling_enabled).toBe(false);
  });

  it("preserves concentrated scheduling when quota bypass is enabled", () => {
    const form = {
      quota_bypass_enabled: true,
      quota_bypass_concentrated_scheduling_enabled: true,
    };

    normalizeQuotaBypassScheduling(form);

    expect(form.quota_bypass_concentrated_scheduling_enabled).toBe(true);
  });
});
