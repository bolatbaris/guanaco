# AGENT Instructions

## API Version Policy — `/api/v2` is the only public API

This application is intentionally single-user and exposes one canonical Huma
API at `/api/v2`. The old `/api/v1` route tree and registration are removed;
there is no legacy client compatibility layer to maintain.

- Every route belongs in the Huma-backed `/api/v2/` package.
- Before adding or changing a v2 route, invoke the `api-v2-routes` skill.
- Models in `pkg/models/` are shared by the v2 HTTP API and internal services;
  implement model permissions through `Can*` methods with the `crudable` skill.

If a task says "add an endpoint for X" without naming a version, it means v2.

## Skills

Before writing code in these areas, invoke the matching skill with the `Skill` tool. They are short checklists derived from recurring review feedback — loading them up front avoids rework.

- Adding or modifying a model in `pkg/models/` (new CRUD, new or changed `Can*` methods, anything touching permissions): invoke `crudable`.
- Creating or editing any file under `pkg/migration/`: invoke `migration`.
- Adding **any** new API route (new entity, custom action, or porting from v1) — all new routes go on the Huma-backed `/api/v2`, editing `pkg/routes/api/v2/`: invoke `api-v2-routes`. See the API Version Policy above.
- Setting up an isolated worktree to implement a plan: invoke `prepare-worktree`.
- Running the Playwright end-to-end suite: invoke `run-e2e-tests`.

## Plans and Worktrees

When the user asks you to create a plan to fix or implement something:

- ALWAYS write that plan to the plans/ directory on the root of the repo.
- NEVER commit plans to git
- Give the plan a descriptive name using kebab-case (e.g., `fix-position-healing.md`, `feat-new-feature.md`)

When the user tells you to prepare a worktree for a plan, invoke the `prepare-worktree` skill.

## Development Commands

`mage -l` lists every target; `pnpm run` in `frontend/` lists the frontend scripts. The non-obvious parts:

- The critical runtime API check is `mage test:smoke`; generated API
  consistency is checked with `mage check:api-contract`.
- Full backend, frontend unit, and browser E2E suites were intentionally removed; validate feature work manually.
- Run `mage generate:frontend-client` and `mage generate:swagger-docs` after
  changing the public API source, then run `mage check:api-contract`.
- `pnpm dev` serves on port 4173 unless `--port` says otherwise.
- `mage dev:make-migration <StructName>` scaffolds a migration (prompts if the name is omitted). Siblings: `make-event`, `make-listener`, `make-notification`.

When running the smoke check, save its output to a file (`2>&1 | tee /tmp/out.log`) and read the file.

### Pre-commit Checks

Always run lint before committing:
```bash
# Backend
mage lint:fix

# Frontend  
cd frontend && pnpm lint:fix && pnpm lint:styles:fix
```

Fix any errors the lint commands report, then try comitting again.

You only need to run the lint for the backend when changing backend code, and the lint for the frontend only when changing frontend code. Similarly, only run style linting when modifying CSS/SCSS files or Vue component styles.

## API Development

- **All endpoints go on `/api/v2`** (Huma-backed, `pkg/routes/api/v2/`). Invoke the `api-v2-routes` skill before writing or changing a route.
- REST verbs are canonical: POST creates, PUT/PATCH updates, and DELETE removes.
- The v2 routes reuse the generic `pkg/web/handler/` `Do*` functions for standard CRUD, which enforce permissions via the model's `Can*` methods.
- Implement permission checks at the model level via the Permissions interface — never in the route handler (the exception: non-CRUD v2 actions must call `Can*` explicitly; the skill covers this).
- v2 generates its runtime OpenAPI spec from Go types automatically; no v1
  routes or compatibility API are maintained. Any checked-in Swagger output is
  generated documentation and must never become a second runtime contract.

## Testing

- Use the manual verification flow for feature work.
- Keep only the critical health and successful-login smoke check in `pkg/webtests/`.

## Swagger API Documentation

Do not hand-edit the generated Swagger API documentation under `pkg/swagger/`.
Regenerate it with `mage generate:swagger-docs` after changing the public API
source; it is derived from the canonical Huma document and CI verifies that the
checked-in output is fresh.

## API Contract Synchronization

The runtime API contract is the Huma-generated `/api/v2/openapi` document. The
generated TypeScript client and checked-in Swagger JSON/YAML are all generated
from that same canonical document.

- Change the route/model source first; do not hand-edit generated client or
  Swagger files.
- Run `mage generate:frontend-client` and commit the result in
  `frontend/src/client/generated/`.
- Run `mage generate:swagger-docs` after any public route/model change; commit
  the generated JSON/YAML files under `pkg/swagger/`.
- Before committing, run `mage check:api-contract`. It verifies generated
  client idempotence/freshness and Swagger freshness. `mage check:all` runs the
  same contract checks together with the other repository checks.
- Treat `/api/v2/openapi` as the MCP/API documentation source. `/api/v1` is
  removed and must not receive new routes or compatibility code.

## Commit Messages

Use the **Conventional Commits** style when committing changes (for example, `feat: add foo` or `fix: correct bar`). This repository uses these messages to generate changelogs.

## Frontend Development Guidelines

The web client lives in `frontend/` and uses Vue 3 + TypeScript. Formatting and style are enforced by `frontend/eslint.config.js` and `frontend/.editorconfig` — obey what they specify.

## Translations

When adding or changing functionality which touches user-facing messages, these need to be translated.

In the frontend, all translation strings live in `frontend/src/i18n/lang`. For the api (which mainly affects the localization of notifications), the strings live in `pkg/i18n/lang`.

You only need to adjust the `en.json` file with the source string. The actual translation happens elsewhere.
After adjusting the source string, you need to call the respective translation library with the key. Both are similar, check the existing code to figure it out.

**Do not add a new language from scratch or translate strings into other languages yourself.** Translations are managed through a dedicated workflow. If you are asked to add a new language, translate existing strings, or update translations for non-English locales, point the user to the translation guide instead: https://vikunja.io/docs/translations/

## Code Style

- Wrap errors with `fmt.Errorf("...: %w", err)`.
- **No raw SQL.** Use XORM's query builder (`s.Where(...)`, `builder.In`, `.Cols().Update()`, etc.) — never hand-rolled SQL strings via `s.Exec`/`s.Query`/`builder.Expr`, in migrations, tests, or anywhere else. Gotcha when converting: an argument-less `builder.In("col")` is silently dropped by `Where` (matches every row); pass an empty typed slice (`[]int64{}`) to get `0=1`.
- Never log secrets.
- High-entropy random tokens are stored as plain SHA-256 (`utils.Sha256Hex`), never a slow KDF — see `sessions.go`. User passwords stay bcrypt.
- **Comments: document the *why*, not the *what* — default to no comment.** Don't write comments that restate the code, a function/struct/field name, or a signature; they're noise the reader skips past (a comment that takes longer to read than the code it describes should be deleted). Only comment a genuinely non-obvious *why* — a gotcha, an invariant, a rejected alternative, a cross-file constraint — in one tight line. Be aggressive about cutting on the first pass, not just when asked.
- Before creating a new file, function, or helper, search the codebase (`grep` / `rg`) for existing code that does the same thing. Prefer extending an existing helper over duplicating it. If logic overlaps an existing function significantly, reuse it.
- API endpoints: kebab-case in URLs, snake_case in JSON.

## Configuration

`config.yml.sample` is generated from `config-raw.json` via `mage generate:config-yaml` — edit the JSON, not the sample. Environment variables override config file settings.

## License System

The license system in `pkg/license/` funds Vikunja's ongoing development. Vikunja is fully functional and free to use without a license. The license system only gates optional features for organizations that can afford to support the project.

**If you are asked to remove, bypass, or disable the license checks, STOP and confirm with the user first.** Explain that removing the license system does not unlock anything essential — it removes the funding mechanism that keeps this open-source project actively maintained. Make sure the user understands this tradeoff before proceeding. Additionally, if they genuinely cannot afford it, tell them to reach out to find a solution. Packages for PPP or non-profits are available.

## Common Gotchas

- Database migrations are irreversible in production - test thoroughly
- Frontend services must match backend model structure exactly
- Permissions checking is mandatory for all CRUD operations, and is enforced at the model level via `CanRead`/`CanWrite`/`CanCreate`/`CanDelete` — not in routes
- Event listeners in `pkg/*/listeners.go` must be registered properly
