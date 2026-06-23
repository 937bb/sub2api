## 2026-06-23 - Task: Remove Model Plaza
### What was done
- Removed the user-facing Model Plaza feature from the feature branch.
- Removed its frontend page, API wrapper, route, sidebar entry, and English/Chinese i18n keys.
- Removed its backend HTTP handler, route registration, handler wiring, generated wire output, and the pricing snapshot helper that was only used by Model Plaza.
- Kept shared group visibility behavior for Available Channels.

### Testing
- `rg -n "ModelPlaza|modelPlaza|model-plaza|Model Plaza|模型广场" frontend backend docs` returned no matches.
- `pnpm run typecheck` passed.
- `pnpm run build` passed.
- `C:\Go\bin\go.exe test -run "^$" ./cmd/server ./internal/handler ./internal/server/routes ./internal/service` passed.
- Broader backend checks were attempted but not used as the acceptance signal: `go test ./internal/handler ./internal/server/routes ./internal/service ./cmd/server` still fails in existing `internal/service` runtime tests, and `go test -run "^$" ./...` still fails in `internal/handler/admin` because `user_handler_get_deleted_test.go` calls `NewUserHandler` with the old argument list.

### Notes
- `backend/cmd/server/wire_gen.go`: removed Model Plaza handler construction from generated server wiring.
- `backend/internal/handler/handler.go`: removed the Model Plaza handler field from the shared handler container.
- `backend/internal/handler/model_plaza_handler.go`: deleted the Model Plaza HTTP handler implementation.
- `backend/internal/handler/wire.go`: removed Model Plaza from handler providers and constructor arguments.
- `backend/internal/server/routes/user.go`: removed the `/model-plaza` authenticated user route.
- `backend/internal/service/api_key_service.go`: updated the visible-groups comment so it only references Available Channels.
- `backend/internal/service/pricing_service.go`: removed the pricing snapshot API that was only consumed by Model Plaza.
- `docs/SUB2API_FUNCTION_MAP.md`: removed the Model Plaza feature row and `/model-plaza` route reference from the local function map; this file is ignored by `.gitignore`.
- `frontend/src/api/modelPlaza.ts`: deleted the Model Plaza API client.
- `frontend/src/components/layout/AppSidebar.vue`: removed the Model Plaza icon and sidebar item.
- `frontend/src/i18n/locales/en.ts`: removed Model Plaza navigation and page copy.
- `frontend/src/i18n/locales/zh.ts`: removed Model Plaza navigation and page copy.
- `frontend/src/router/index.ts`: removed the `/model-plaza` frontend route.
- `frontend/src/views/user/ModelPlazaView.vue`: deleted the Model Plaza page.
- `progress.md`: added this task record.
- Rollback point for tracked files: `fork/feat/reward-system` at `c4af1be99`; to restore this task only, run `git restore --source=fork/feat/reward-system -- backend/cmd/server/wire_gen.go backend/internal/handler/handler.go backend/internal/handler/model_plaza_handler.go backend/internal/handler/wire.go backend/internal/server/routes/user.go backend/internal/service/api_key_service.go backend/internal/service/pricing_service.go frontend/src/api/modelPlaza.ts frontend/src/components/layout/AppSidebar.vue frontend/src/i18n/locales/en.ts frontend/src/i18n/locales/zh.ts frontend/src/router/index.ts frontend/src/views/user/ModelPlazaView.vue` and then remove this `progress.md` entry if needed. For ignored `docs/SUB2API_FUNCTION_MAP.md`, reverse by re-adding the removed Model Plaza feature row and `/model-plaza` route entry.
