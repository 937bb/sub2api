# Revision 8 verification

This frontend-only revision extends the user-pinned New API visual language through shared overlays and their interiors. Sub2API backend code, API payloads, billing, routing, permissions and data remain authoritative. The C application is not a deployment target.

## Automated checks

- Frontend full suite after reviewer corrections: 377 files, 2,902 tests passed; `/tmp/b-console-ui8-tests-reviewfix.log`.
- Frontend typecheck and changed-source ESLint passed; `/tmp/b-console-ui8-types-reviewfix.log`, `/tmp/b-console-ui8-eslint.log`.
- Locale key completeness is included in the full frontend suite.
- Production frontend build passed; `/tmp/b-console-ui8-build-reviewfix.log`. The existing large-chunk advisory remains.
- Backend full tests and vet passed; `/tmp/b-console-ui8-backend-tests.log`, `/tmp/b-console-ui8-vet.log`.
- Ordinary backend compilation and embedded web tests passed. Final Linux embedded compilation is a release gate after the commit is fixed.
- `git diff --check` passed. Backend, migrations, deployment manifests and dependency lockfile have no source changes.

## Interaction and visual scope

See `review/revision8-coverage.md` for the complete source inventory and the distinction between directly exercised families and shared primitive coverage. Captures are local previews with synthetic data, at desktop and 390px mobile, including dark confirmation/image surfaces. No real payment or administrative data was created to obtain screenshots.

The added focus tests cover nested dialogs, forward/backward tab containment, body lock, focus restoration including conditional unmount, selector portals and the real shared image preview. Existing payment, announcement and OTP assertions remain in the suite.

The review correction batch unified the custom Select trigger with the shared filled fields and added localized accessible naming to user creation/editing, password actions, step-up OTP digits and the shared close button. Browser DOM/AX inspection confirmed these names, matching native/custom control background and radius, initial OTP focus and return to the parent trigger on cancellation. Corrected desktop/mobile captures use the same evidence paths; additional edit/redemption mobile captures and `reviewfix-a11y.json` record the focused verification.

The detector ran once. Its 40 `gray-on-color` findings were supplied to the independent finish reviewer with source and screenshots. Static adjacency warnings are not automatically proof of low rendered contrast.

## Deployment boundary

Use the existing B database and data mount, retain previous content-addressed frontend assets for open tabs, start and validate a candidate, switch the 6064 proxy without replacing its bind-mounted inode, and prove C-origin requests reach the candidate. Keep rollback listeners for existing streams. Recheck data counts, migrations, both public domains and at least five minutes of health/error evidence. Final release results belong in the release report, not in this predeployment checklist.
