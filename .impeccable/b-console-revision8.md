# Revision 8 — complete interaction surfaces

Mode: Operate. User-pinned New API frontend; Sub2API remains the backend. The user rejected revision 7 because modal interiors, drawers and secondary surfaces retained the previous product's layout. The inherited direction seed is 1f6e3f1a; no new seed provenance or approved comp is claimed.

THESIS: every task opens in the same quiet, compact component system, including body-mounted portals. Form labels sit above controls; related fields form real sections; long editors use proportionate right-side sheets; confirmations remain short centered dialogs.

FIRST VIEWPORT: a compact title and close control, clearly named field groups, neutral filled controls and short help text. Footer actions remain visible while only the body scrolls. A small form must not occupy an 880px strip with per-field horizontal rules.

Reference evidence: QuantumNous/new-api main, components/dialog.tsx and drawer-layout.ts (read 2026-10-27), plus the user's New API reference and prior usage screenshot. The Vue implementation uses the reference's hierarchy and interaction conventions, not its React dependencies or backend. No New API account, model, billing or permission behavior is imported.

Implementation boundary: shared controls, dialogs, drawers, popovers, forms, detail grids, notification/security/payment overlays, and their callers. Preserve handlers, validation, API payloads, permissions, currencies and precision. Do not touch inference, billing or C → 6064 routing.

Verification: inventory every BaseDialog and standalone modal/portal; cover form, editor, confirmation, detail, nested overlay, security, announcement and payment families. Capture populated local previews at desktop/mobile and dark theme, including focus, scrolling and menus. Backend/data validation and guarded deployment use the release runbook. A source inventory is not a claim that every conditional branch was visually exercised.
