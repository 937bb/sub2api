# 937Sub B console

The B service is an API gateway with both ordinary customers and administrators. Customers manage API keys, inspect usage and charges, and access their enabled subscription and payment features. Administrators manage users, upstream accounts, groups, pricing, monitoring, and usage investigations.

This redesign targets the B console and its shared authentication layout. The customer rejected a minor restyle and explicitly requested a structural replacement of navigation, charts, and page layouts. Their preferred interface is clean and professional: primarily light, compact, and focused on important data. Keep all existing permissions, feature flags, API contracts, billing semantics, branding settings, and navigation destinations. C is a separate deployment and is outside this change.

Production source baseline: b1b5de679c643148738b833281781b59228409cc. UI work is isolated in codex/b-console-ui-20261027. Preview figures are synthetic and are not operational evidence. Deployment is a separate task.
Confirmed visual reference (revision 4): Stripe — light surfaces, clear hierarchy, restrained blue-violet accents. One sidebar, consistent shared controls, horizontal on-demand filters, and content-led layouts. This replaces the rejected dark top bar and teal action styling.
