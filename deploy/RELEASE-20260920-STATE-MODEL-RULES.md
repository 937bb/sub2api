# Codex state model rules: 0.2.6.17

The Codex state page now supports editable target-length rules per subscription
plan and upstream model. Defaults are Pro/all models `[292]`, Team/gpt-5.6-terra
`[286]`, and Team/gpt-6-astra `[273]`. Unmatched plans/models retain the existing
fallback `[332,292]`. Lengths are local preferences, not evidence of model quality.

Rules resolve in this order: exact plan/model, exact plan with wildcard model,
wildcard plan with exact model, both wildcards, global fallback. Legacy settings
without rules receive the defaults; an explicit empty array removes all rules.
Administrators can add, edit, and remove rules without restarting the service.

Scanning, persisted readiness, summaries and outbound selection share this
policy. Values remain isolated by credential-owning account and upstream model.
Pro 5x (`prolite`) and Business Premium aliases map to Pro and Team respectively.
When imported credentials omit a plan, explicit Team verification is used only
if its verified workspace matches the credential's current ChatGPT account ID.
Account names do not infer a subscription tier. Valid values suppress scans
until the refresh window. Rule edits invalidate preferred-state indexes.

Opaque values with a length of 273 no longer fail merely because their complete
suffix is not Base64-decodable. Timestamp-prefix parsing still requires the
version marker and a URL-safe alphabet. It does not authenticate a token or
renew its expiry; expired and future-issued values remain unusable.

Validation includes scoped scanner/pool tests, frontend rule editing and
independent scan-action tests, plus real PostgreSQL policy/readiness and lease
binding tests in an isolated database (`SUB2API_STATE_TEST_DSN`). Production
rollout retains port 6064 and serves the new image before draining the old one.
Keep old hashed frontend assets available to avoid stale-browser asset 404s.
