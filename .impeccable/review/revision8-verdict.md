## verdict

1. **Resolved — shared Select fidelity.** Reopened the exact previously reviewed `round1-user-edit-1440.png`, `final-user-create-1440.png`, `final-user-create-390.png`, `final-redeem-1440.png`, and `final-nested-1440.png`, plus requested `final-user-edit-390.png` and `final-redeem-390.png`; all are valid. User-edit and redemption custom triggers now visibly use the same neutral filled surface and compact corners as native fields at desktop/mobile. `Select.vue` now directly uses shared background, border, radius, padding and focus tokens; the competing white/dark background and scoped ring declarations are removed. Existing option/search and disabled/error interfaces remain present in the scoped source change.
2. **Resolved — accessible naming.** The same recaptures preserve readable visible labels, unclipped field layout, footer actions and nested OTP focus. Source confirms stable `useId()` associations for every previously flagged create/edit field, explicit custom-role and password-action names, localized digit names with the shared OTP instruction relationship, and the localized shared close label with isolated-consumer fallback. `reviewfix-a11y.json` records six correctly named/described OTP inputs, focus on digit one, and cancellation returning focus to “验证并保存” while retaining the parent scroll lock. Create/edit accessible-name browser verification and the repeated passing suite are parent-provided evidence; this reviewer performed source/capture review, not a new live test.

No material regression from this fix batch is visible in the scoped recaptures or inspected changes. No browser, detector, test run, application edit, or new issue hunt was performed.

## remaining

Clear. This ship verdict covers the two scored fixes, not the whole surface. The original review's representative-coverage, provenance and conditional-state limits remain unchanged.

disposition: ship
