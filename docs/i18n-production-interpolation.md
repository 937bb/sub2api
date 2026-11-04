# Production translation interpolation repair

The application loads TypeScript locale modules as raw strings. Both Vite and Vitest previously aliased Vue I18n to the runtime-only bundle, without a locale precompilation plugin. As a result, named parameters such as `{name}`, `{model}`, `{seconds}`, `{count}` and `{cost}` appeared literally in the UI. Existing key-completeness and standalone message-syntax checks did not exercise the production message compiler registration.

Both resolvers now select the compiler-inclusive bundle already present in the existing Vue I18n dependency. No package versions or lockfile change. The build explicitly enables AST JIT and retains the message compiler; this repair does not loosen Content Security Policy. The existing server dependency directory has Vue I18n 9.14.5, while the source manifest declares 11.x. Explicit JIT also supports the older installed build without upgrading unrelated production dependencies.

`pnpm test:i18n-production` loads the actual Vite production configuration, bundles both real locales and representative account-test/list messages, and asserts their final rendered strings. The resulting minified bundle runs in a VM that rejects string-based code generation. The check fails with the original runtime-only alias and passes with the repaired alias. `pnpm build` runs it after the application build.

The test uses a virtual entry and `write: false`, so it cannot overwrite production assets. The release retains prior hashed assets for already-open pages. Existing pages need one reload to load the corrected bundle.
