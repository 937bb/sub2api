# B console — revision 5
Mode: Operate. Code-led replacement; no approved image comp.
User reference: Stripe — light surfaces, clear hierarchy, restrained blue-violet accents.
Scope: B administrator and user console, shared authentication; preserve permissions, feature flags, payments, billing, API contracts, and content. C and deployment are outside this change.

FIRST VIEWPORT: one 232px light module sidebar, a quiet 60px breadcrumb header, and a white working surface. Clear 24px page title and contextual actions precede a compact summary or useful ledger. No dark horizontal business navigation, second title, or permanent collection filter column. Main actions use primary600 (#635bce); semantic colours indicate status, not arbitrary provider CTAs.

Signature interactions: permission-aware navigation search and collapse; on-demand horizontal filters preserving query state; existing table selection and batch actions; analytics metric switching with clear units and ranked model bars. Existing actions remain reachable.

Cross-surface grammar: white workspace (#fff), quiet navigation gray (#f6f7fa), neutral dividers, 36px shared controls, and consistent compact spacing. Collection ledgers, reports, and configuration documents use flat divided sections; standalone checkout, identity, subscription, and overlay containers retain 12px corners. Settings put section headings above fields. Checkout separates editable form and order summary; plans use compact horizontal rows. Mobile becomes one column with a navigation drawer. Shared useDark theme state preserves stored dark mode.

QUALITY BAR: familiar professional SaaS clarity at Stripe's level of hierarchy and restraint; content density without miniature type, consistent input/button/select/action language, no competing decorative action colours. Charts remain legible and distinguish quantities without rainbow fill. Synthetic preview data is marked as demonstration, never operational evidence.

Revision 4 was rejected based on production screenshots. Its verification does not establish acceptance of this revision.

## Direction contract

THESIS: 937 uses three task structures: a collection ledger, an analytical report, and a configuration document. Replace inherited card stacking and all-menu navigation with these repeatable structures.

OWN-WORLD: retain the explicitly chosen Stripe reference, light neutral surfaces and violet actions. A white working surface, quiet module sidebar, compact query toolbar, and tabular numerical rhythm identify the console. Preserve configured branding; use 937 Console for the stock-name fallback.

STORY: choose administration or personal work, expand the relevant business module, then operate on records or read a report. Search still spans all authorized destinations. Filters preserve state and never consume the desktop viewport.

FIRST VIEWPORT: 232px single sidebar with workspace switch and up to six authorized expandable modules; a 60px breadcrumb header; title and actions; a compact record toolbar followed by horizontal query fields and an unboxed ledger. Analytics use a 1.8:1 report and ranking composition, with content-sized sections and 24–32px separation. Workspace switching is for non-simple-mode administrators; simple-mode and administrator custom routes preserve their administrator context.

FORM: task-led collection/report/document structure; surface seed b4177515. The user's pinned Stripe choice and concrete rejected screenshots govern the form. No further palette-choice round is required. Signature behavior is module expansion plus inline query reveal; portals are constrained to the viewport rather than the parent panel.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

Verification: 1440px desktop, 1750px screenshot-equivalent desktop, 1024px, 390px mobile, 320px short-window version panel, dark mode, admin and ordinary user. Fixtures include prefixed long versions, one payment method and ten ranked users, and multiple currencies. Verify open filters and actual bounds, not only document overflow. Independent review includes the user's rejected screenshots.

Revision-5 implementation and bounded local evidence are recorded in DESIGN.md, .impeccable/design.json, and .impeccable/verification-revision5.md. This coverage statement is a verification target; the evidence record identifies actual captures and limits. Revision-4 captures do not establish acceptance. Deployment requires its own release record.
