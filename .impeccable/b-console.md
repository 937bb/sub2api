# B console — current surface brief

Mode: Operate. Revision 6 is current. The authoritative direction contract is [b-console-revision6.md](b-console-revision6.md); the extracted system is [../DESIGN.md](../DESIGN.md) with [design.json](design.json).

The user explicitly replaced rejected revision-5 Stripe with New API. Current structure: 60px full-width header, 208px directly categorized sidebar (64px collapsed), light gray canvas, rounded white workspace, default-open collection filters, bordered tables with pagination, five compact measures, and full-width metric/form-switching analysis. Existing nested feature navigation remains expandable. Mobile uses a drawer and stacked work areas.

Scope: B administrator/customer console and shared authentication. Preserve configured branding, permissions, feature flags, destinations, API contracts, billing distinctions, and operational behavior. C and deployment remain separate. Product truth lives in [../PRODUCT.md](../PRODUCT.md).

Evidence is local and synthetic. Current captures live in screenshots/revision6/final-*.png; the bounded finish review is [review/revision6-finish.md](review/revision6-finish.md). The initial review requested simple-mode financial-visibility correction, readable group period metadata, and design-document persistence. Current source contains the first two corrections; DESIGN.md and design.json now resolve the persistence finding. Post-fix capture/test status belongs to the implementation handoff and review evidence, not an assumed all-route pass.

The documenter inspected current styles/layout/chart/usage source and sampled final dashboard/groups captures. Custom-route evidence is an empty configured page; screenshots are not authenticated production evidence. The implementation handoff reports post-correction success for 376 frontend test files / 2,896 tests, vue-tsc, ESLint, the production Vite build, and Go embedded-web tests. Simple-mode 1440, groups 1440/1750/390, dashboard 1440, and dark dashboard 1024 captures were recaptured and opened after the fixes. These are parent-reported checks, not independent documenter reruns or universal route coverage.

Reference provenance: QuantumNous/new-api official dashboard.en.jpg and usage-logs.en.jpg screenshots, viewed 2026-10-27. Visual reference only; no source transplant or newly introduced raster asset.
