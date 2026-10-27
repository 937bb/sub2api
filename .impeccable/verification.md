# B console UI verification — revision 4

Baseline: production source `b1b5de679c643148738b833281781b59228409cc`.
Working branch: `codex/b-console-ui-20261027`.
User reference: Stripe — light surfaces, clear hierarchy, restrained blue-violet accents.

## Scope

B console frontend for administrators and ordinary users, plus shared authentication. See coverage.md. No backend, API contract, billing calculation, database, gateway, C deployment, or server configuration changed. This is a local implementation; no commit, push, or production deployment performed.

## Verified results

- Broad relevant Vitest regression: 855 passed, zero failed (`revision4-tests.json`).
- Focused shared controls, layout, payment and profile regression: 338 passed, zero failed (`revision4-final-tests.json`). These overlap the broad run; they are not 338 additional unique tests.
- Final i18n and operations component tests: 52 passed, zero failed (`revision4-i18n-ops-tests.json`).
- Typecheck (`vue-tsc --noEmit`, then `vue-tsc -b`) and Vite production build passed. Existing bundle-size, mixed-import and stale Browserslist warnings remain.
- ESLint: 143 changed Vue/TypeScript/JavaScript files, zero findings (`revision4-lint.json`). Final AnnouncementBell, DashboardView and OpsDashboardHeader changes checked again with no findings.
- `git diff --check`: clean.
- Browser route matrix: 31 unique authenticated routes, 90 desktop/mobile/dark route and settings-tab checks; no page errors, Vite overlays or document horizontal overflow (`revision4-confirm.json`). Authentication captured separately.
- Interaction harness: navigation search and Escape clearing, sidebar collapse/icon access and expand, mobile navigation, filter hide/show retaining query, account sheet Escape, audit filter fit, mobile payment settings, batch create with filters hidden all passed.
- Computed main action colour equals `rgb(99, 91, 206)`; palette validated after restarting the local Vite process to invalidate stale Tailwind config.
- Current evidence is `screenshots/revision4/confirm-*`; earlier revision evidence and inspect captures are superseded.
- Review corrections: 20 chart/plan tests passed; five edited Vue files passed ESLint; typecheck and production build passed again. Ten affected route renders were recaptured with zero overflow/errors (`revision4-recapture.json`).
- Live portal check: visible account-sheet input and textarea placeholders compute to `rgb(100, 116, 139)` with opacity1, approximately 4.76:1 on white. Plan labels and standard-cost values use gray600/dark400; subscription renewal uses the shared primary button.

## Automated detector

Ran once with the skill launcher through sh, across 98 changed UI source files, without importing the superseded revision3 design system. Raw findings: `revision4-detector.json` (48).
Fixed mechanical width/margin animations, decorative announcement gradients, and thick toast/tour borders. Most remaining grey-on-colour hits describe mutually exclusive disabled/selected branches in template strings; markdown blockquote selectors and unused/public gradient helpers are existing specialized patterns. Independent review assesses remaining material findings. The detector is not an accessibility certification.

## Independent review

Full revision4 review returned `fix` for two material findings: insufficient placeholder/information contrast and the provider-coloured subscription renewal CTA. Both were corrected in one batch. Verdict pass returned `ship`, scoring both findings resolved with no regression observed in the recaptured surfaces. This verdict covers those two fixes; it is not a whole-product accessibility certification or production acceptance. See `review-v4.md`. Revision3 approval is superseded by the user's rejection and does not authorize revision4 acceptance or deployment.

## Reproduce

Start Vite on loopback port3197 with frontend dependencies installed. Use a real Node binary if the local Volta pnpm shim fails.

```sh
REVISION=revision4 PLAYWRIGHT_MODULE=/absolute/path/to/playwright \
node .impeccable/preview-v3.cjs confirm
REVISION=revision4 PLAYWRIGHT_MODULE=/absolute/path/to/playwright \
node .impeccable/verify-interactions-v3.cjs
REVISION=revision4 PHASE=confirm node .impeccable/contact-sheets.cjs
```

The harness intercepts APIs using synthetic users, usage, and settings; external requests are blocked. It does not use production credentials or add a preview/authentication bypass to product code. Preview data is not operational evidence.

## Limits

Synthetic checks do not validate real payment/provider/OAuth writes. No actual recharge or account mutation was submitted. Specialized provider-owned forms, OAuth callbacks, public legal/marketing pages and the initial setup wizard retain their existing flows. This is not a full screen-reader audit or production rollout validation.
