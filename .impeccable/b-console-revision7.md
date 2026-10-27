# B console — revision 7

Mode: Operate. Continued code-led New API redesign. Revision 6 was explicitly rejected: too far from New API, intrusive violet, poor operations composition, and dated usage records. The supplied operations screenshot is the rejection baseline. No approved comp exists. Preserve the existing direction seed 1f6e3f1a; the user-pinned New API reference governs.

THESIS: make requests and operational trends the working surface. Compact query controls, aligned records, and restrained status notation replace oversized nested panels.

OWN-WORLD: neutral gray canvas, white work surfaces, graphite text, muted blue actions. Violet is not a category color. Use readable 12px metadata, 13px controls and table data, and consistent tabular numerals. Keep meaningful warning, error and success states.

STORY: select a destination; query the collection; read the results; inspect or act. Secondary query dimensions can expand without clearing them. An active-filter count keeps hidden criteria discoverable. Analysis remains one explicit view away.

FIRST VIEWPORT: compact heading and query toolbar, a short aggregate strip, then request rows. Operations starts with a full-width diagnosis disclosure and eight aligned metric groups, a short resource band, and visible analysis. No empty column under realtime traffic.

FORM: New API's compact operational hierarchy is the reference, not its brand. Preserve all destinations, filters, actions, stored column choices, tooltips, exact billing fields and units, and admin/customer/simple-mode boundaries. Source colors and component structure must agree with shared tokens; CSS recoloring alone is insufficient.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance.

Verify synthetic populated and absent-data views at 1440, 2048, 1024 and 390px, light/dark, operations normal/fullscreen, administrator/customer records, and representative shared page families. Production deployment changes B frontend only, preserving the API, data mounts, running streams, and C → 6064 contract.

## Implemented documentation addendum

The revision-7 visual system is extracted in [DESIGN.md](../DESIGN.md), with normative YAML tokens and the eight canonical design sections. [.impeccable/design.json](design.json) is its schemaVersion 2 sidecar: tonal-ramp metadata, overlay shadows, motion, breakpoints, and self-contained component previews. [PRODUCT.md](../PRODUCT.md) holds durable product constraints. This refresh was part of the repeatedly requested replacement redesign; it does not record final user acceptance.

Source authority is frontend/tailwind.config.js and frontend/src/style.css for the primary ramp and primitives, frontend/src/styles/console.css for theme, shell, usage, and common surfaces, and frontend/src/styles/workbench.css for page composition. The action ramp is muted blue, neutral surfaces carry the work, and decorative violet has been removed from the redesigned shared families. Genuine Stripe/provider branding remains payment-specific.

Operations implements eight groups in four columns by two rows at 1000px and above, two columns at 999px and below, and one at 599px and below. Realtime QPS/TPS uses aligned current/peak/average rows. The resource band uses six, three, then two columns at the same thresholds. Throughput and concurrency share the first desktop analysis row; switching spans the second. Analysis stacks below 1024px. At 1000–1199px, title/status and compact control gaps preserve toolbar space. Throughput and switching charts react to theme changes.

Usage records implement query → aggregates → records/pagination, with a separate analysis view. Advanced filters retain their values when hidden and show an active count. Below 480px, the records aggregate strip collapses behind a summary toggle; paired primary fields keep a complete primary record compact, and More preserves selected secondary columns and their detail. Generic collections, settings, account/profile, payment, and other shared families use flat page roots with individual working regions rather than nested page-wide cards.

Provenance: inherited direction seed 1f6e3f1a; no independent revision-7 seed-run evidence; no approved comp; reference-driven, code-led implementation based on the user's New API reference. Current captures are under screenshots/revision7/. They contain synthetic local preview data and are not authenticated production sessions. The documenter sampled final-ops-1440.png and final-usage-390.png; broader checks and final capture ownership remain with the finish review. The parent implementation pass reported 376 test files / 2,899 tests passing, plus typecheck, ESLint, and the production build. This documentation pass did not rerun app checks or deploy.

The detector ran once and reported an untouched announcement blockquote. That finding is recorded rather than adopted as a visual rule. No new generated raster artwork was added; configured logos, provider assets, and reference screenshots retain their distinct provenance. Finish verdicts and test results establish implementation evidence, not final user approval.
