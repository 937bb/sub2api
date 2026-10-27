# 937Sub B console

The B service is an API gateway with both ordinary customers and administrators. Customers manage API keys, inspect usage and charges, and access their enabled subscription and payment features. Administrators manage users, upstream accounts, groups, pricing, monitoring, and usage investigations.

This redesign targets the B console and its shared authentication layout. The customer rejected a minor restyle and explicitly requested a structural replacement of navigation, charts, and page layouts. Their preferred interface is clean and professional: primarily light, compact, and focused on important data. Keep all existing permissions, feature flags, API contracts, billing semantics, branding settings, and navigation destinations. C is a separate deployment and is outside this change.

Production source baseline: b1b5de679c643148738b833281781b59228409cc. UI work is isolated in codex/b-console-ui-20261027. Preview figures are synthetic and are not operational evidence. Deployment is a separate task.
Confirmed visual reference (revision 6): New API. The customer rejected revision 5 as still resembling Sub2API. Replace its expandable-module sidebar, flat ledgers, three-column summaries, and side-by-side analysis with a global header, directly visible categorized navigation, framed data workspaces, compact multi-metric summaries, and full-width analytical panels. Preserve all existing operational behavior. New API is a visual reference, not a source-code transplant or a change of backend.
