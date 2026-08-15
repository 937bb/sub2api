export interface QuotaBypassSchedulingFormState {
  quota_bypass_enabled: boolean;
  quota_bypass_concentrated_scheduling_enabled: boolean;
}

export function normalizeQuotaBypassScheduling(
  form: QuotaBypassSchedulingFormState,
): void {
  if (!form.quota_bypass_enabled) {
    form.quota_bypass_concentrated_scheduling_enabled = false;
  }
}
