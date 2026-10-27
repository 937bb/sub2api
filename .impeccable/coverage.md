# B console revision 4 coverage

Local frontend replacement, 2026-10-27. Production and C are not modified. All preview records are synthetic, not production metrics.

| Surface family | Structural replacement | Verification |
| --- | --- | --- |
| Authenticated shell, all admin/customer routes | Single searchable grouped sidebar; compact utility header; restrained violet actions; desktop collapse and mobile drawer | Desktop 1440px, mobile 390px, dark desktop captures; permission-aware original route data retained |
| Accounts, users, groups, channels, proxies | Page-level actions outside filters; horizontal on-demand filters above results; real matching/current-page counts; current-page status breakdown; table density | Populated account/user/group captures, empty-state channel/proxy captures, existing view tests |
| Keys, available channels, batch-image, audit, usage, announcements, redemption codes, promo codes, subscription administration | Shared collection composition with actions/horizontal filters/ledger/pagination | Route captures, existing view tests; specialized table columns and detail content retained |
| Account/user/group/key/proxy editors, subscription assignment, price plans, monitor editor | Opt-in full-height side sheets with compact labeled field rows; footer actions remain visible | Populated account sheet captures; create/edit/bulk tests; BaseDialog behavior preserved |
| System settings including security/payment/backup | Category directory plus top-labeled configuration sections; sticky save action; responsive category tabs | General/security/payment/backup captures; SettingsView and BackupView existing tests |
| Admin/user dashboards and usage analytics | Grouped financial/operational summaries; dedicated analysis plot; metric switching; horizontal model/group/endpoint charts | Populated captures and chart/view tests |
| Operations monitoring | Health diagnosis disclosure; metric ledger; traffic/capacity, latency/errors and activity workspaces; horizontal error distribution | Synthetic throughput and error fixtures; operations tests; captured traffic/quality tabs |
| Channel status V1/V2 and monitor administration | Identity/latency/history rows in V1; flattened monitor heading/controls; two-column configuration workspace in V2 | V1 capture and timer tests; V2 configuration tests; V2 live data not exercised |
| Distillation, risk control, prompt audit | Separated metric strip and configuration/document layouts; shared navigation entry | Distillation capture and view tests; risk/prompt feature-gated components retain logic |
| Recharge, subscription purchase | Selection and order review columns; compact payment methods; horizontal plan comparisons; preserved amounts, fees and currencies | Amount selection capture, plan comparison capture; payment tests; no payment submitted |
| User subscription, profile, redemption, affiliate | Entitlement rows; identity/security/notification tabs with flat identity metadata and editing sections; redemption form/help columns with history below; finance strip and referral/transfer sections | Synthetic populated captures, view tests |
| Payment orders and admin revenue | Collection layout; per-currency finance strip; revenue/orders chart modes | Populated order/revenue captures; existing plan/order tests; no currency aggregation introduced |
| Plugin catalog and custom content | Document/catalog composition; iframe/document viewport preserved | Empty catalog capture and plugin/custom-content tests |
| Login/register/recovery | Shared split identity/form shell | Login desktop/mobile captures; authentication logic unchanged |

## Boundaries

The shared product shell covers authenticated routes. Native input behaviors, permission filtering, feature gates, all protocol/API handlers and all monetary calculations are retained. This is not a gateway, data migration or release operation.

Redirect-only OAuth callbacks, public legal/marketing pages, the initial setup wizard, provider-owned Stripe/Airwallex embedded fields and standalone payment-result/pop-up internals retain their specialized flow. They are not claimed as newly authored screens. Reusable controls inherit the new presentation where mounted inside the console. Tests and synthetic captures do not prove production write workflows or live provider behavior.

## Evidence

Revision 4: `revision4-tests.json` contains 855 passing broad regression tests; `revision4-final-tests.json` contains 338 passing focused tests. `revision4-lint.json`: 143 changed Vue/TS/JS files, zero findings. `revision4-confirm.json`: final route checks. `screenshots/revision4/confirm-*`: current visual evidence. Earlier revisions and inspect captures are superseded. Review scope and limitations are recorded in `review-v4.md` and `verification.md`.
