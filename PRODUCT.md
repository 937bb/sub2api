# 937Sub B console

The B service is an API gateway with both ordinary customers and administrators. Customers manage API keys, inspect usage and charges, and access their enabled subscription and payment features. Administrators manage users, upstream accounts, groups, pricing, monitoring, and usage investigations.

This redesign targets the B console and its shared authentication layout. The customer rejected a minor restyle and explicitly requested a structural replacement of navigation, charts, and page layouts. Their preferred interface is clean and professional: primarily light, compact, and focused on important data. New API is the user-pinned visual reference. Keep all existing permissions, feature flags, API contracts, billing semantics, branding settings, navigation destinations, and stored theme preferences. C is a separate deployment and is outside this change.

Production source baseline: b1b5de679c643148738b833281781b59228409cc. UI work is isolated in codex/b-console-ui-20261027. Preview figures are synthetic and are not operational evidence. Deployment is a separate task.

Revision 7 continues the explicitly requested replacement after the customer rejected the previous violet-heavy look, operations composition, and usage records. Its implemented tokens and shared component rules are recorded in DESIGN.md and .impeccable/design.json; task-specific composition, evidence, and provenance are recorded in .impeccable/b-console-revision7.md. The record does not imply final user acceptance. New API remains a visual reference rather than a source-code transplant or a backend change.

Operational completeness is a product constraint: advanced query dimensions, selected columns, exact billing values and units, account/customer distinctions, error states, and existing actions remain accessible as layouts change. Genuine payment-provider branding, including Stripe, remains recognizable within its payment flow.
