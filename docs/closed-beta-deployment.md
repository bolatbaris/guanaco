# Closed beta deployment

This is the deployment contract for the first closed beta. Keep the service
behind the chosen access layer (for example Tailscale or Cloudflare Access)
until the beta is ready for public traffic.

## Container contract

- The container listens on `0.0.0.0:3456`.
- Configure the health check as `GET /api/v2/health`.
- The image runs as UID `1000`; do not mount writable directories owned only
  by root.
- Persist `/db` only when using SQLite. PostgreSQL is the recommended database
  for the beta.
- Persist `/app/vikunja/files` when using local file storage. If file storage
  is moved to S3, that mount is not required.
- Keep WebSocket upgrades enabled at the reverse proxy; the WebSocket endpoint
  is served under the same port.

## Coolify setup

Use one of these source modes and keep it consistent:

1. Build the `production` branch in Coolify; or
2. Let GitHub Actions publish the image and deploy an immutable image tag
   (`ghcr.io/<owner>/<repository>:<commit-sha>`).

For a closed beta, use the protected exposure layer and TLS. Set the
application port to `3456`, use `/api/v2/health` as the health path, and put
the application and PostgreSQL resource in the same Coolify project and
environment. PostgreSQL should not be publicly accessible. Schedule a
database backup before inviting beta users.

## Runtime environment

Create these as runtime variables in Coolify. They must not be Docker build
arguments and must not be committed to the repository.

```dotenv
VIKUNJA_SERVICE_PUBLICURL=https://beta.example.com/
VIKUNJA_SERVICE_INTERFACE=0.0.0.0:3456
VIKUNJA_SERVICE_SECRET=<openssl-rand-hex-32-output>

VIKUNJA_AUTH_SINGLEUSER_ENABLED=true
VIKUNJA_AUTH_SINGLEUSER_USERNAME=<choose-a-username>
VIKUNJA_AUTH_SINGLEUSER_PASSWORD=<strong-password-at-most-72-bytes>
VIKUNJA_AUTH_SINGLEUSER_EMAIL=<operator-email>
VIKUNJA_AUTH_SINGLEUSER_NAME=<operator-display-name>
VIKUNJA_AUTH_LOCAL_ENABLED=true
VIKUNJA_AUTH_LDAP_ENABLED=false
VIKUNJA_AUTH_OPENID_ENABLED=false

# Sentry-compatible GlitchTip runtime configuration.
# Keep the DSNs empty unless the matching GlitchTip projects are ready.
VIKUNJA_SENTRY_ENABLED=true
VIKUNJA_SENTRY_DSN=https://<api-public-key>@glitchtip.example.com/<api-project-id>
VIKUNJA_SENTRY_FRONTENDENABLED=true
VIKUNJA_SENTRY_FRONTENDDSN=https://<frontend-public-key>@glitchtip.example.com/<frontend-project-id>
VIKUNJA_SENTRY_ENVIRONMENT=beta
VIKUNJA_SENTRY_TRACESAMPLERATE=0.05
VIKUNJA_SENTRY_FRONTENDTRACESAMPLERATE=0.05
VIKUNJA_SENTRY_FRONTENDREPLAYSESSIONSAMPLERATE=0
VIKUNJA_SENTRY_FRONTENDREPLAYSONERRORSAMPLERATE=0

VIKUNJA_DATABASE_TYPE=postgres
VIKUNJA_DATABASE_HOST=<coolify-postgres-internal-host>
VIKUNJA_DATABASE_USER=<database-user>
VIKUNJA_DATABASE_PASSWORD=<database-password>
VIKUNJA_DATABASE_DATABASE=guanaco_beta
VIKUNJA_DATABASE_MAXOPENCONNECTIONS=20
VIKUNJA_DATABASE_MAXIDLECONNECTIONS=10

# The combined image serves the frontend and API from one origin.
VIKUNJA_CORS_ENABLE=false
VIKUNJA_RATELIMIT_ENABLED=true
VIKUNJA_RATELIMIT_STORE=memory
VIKUNJA_LOG_STANDARD=stdout
VIKUNJA_LOG_LEVEL=INFO

# Optional protected metrics/profiling endpoints. Both credentials are required.
VIKUNJA_METRICS_ENABLED=false
VIKUNJA_METRICS_USERNAME=<metrics-scraper-username>
VIKUNJA_METRICS_PASSWORD=<metrics-scraper-password>
VIKUNJA_METRICS_PPROF=false
```

The Sentry settings are a deployment contract for the Phase 1 observability
implementation. Error events remain unsampled; the trace rate defaults to 5%.
Replay is disabled by default and must remain disabled until masking, consent,
retention, and GlitchTip support have been verified. An empty DSN is safe and
never falls back to an upstream Sentry project.

Metrics and profiling are opt-in. If `VIKUNJA_METRICS_ENABLED=true`, both
`VIKUNJA_METRICS_USERNAME` and `VIKUNJA_METRICS_PASSWORD` are required;
otherwise `/metrics` and `/debug/pprof/` remain disabled. Set
`VIKUNJA_METRICS_PPROF=true` only when the protected profiler is needed.

## GlitchTip source maps and CI variables

Source-map upload credentials are build-time CI secrets, not runtime variables.
The frontend build contract uses:

| Variable | Value |
| --- | --- |
| `SENTRY_URL` | The self-hosted GlitchTip base URL, for example `https://glitchtip.example.com` |
| `SENTRY_AUTH_TOKEN` | A GlitchTip release/source-map upload token; keep secret |
| `SENTRY_ORG` | The GlitchTip organization slug |
| `SENTRY_PROJECT` | The frontend GlitchTip project slug |
| `RELEASE_VERSION` | The immutable frontend release identifier supplied by the image build |

The release passed to the uploader must exactly match the release sent by the
browser SDK. The current Vite configuration derives both from `RELEASE_VERSION`.
Upload source maps in CI, then remove them from the deployable frontend artifact.
Never pass `SENTRY_AUTH_TOKEN` as a Docker runtime variable,
embed it in the browser bundle, or commit it (or any password/API token) to the
repository. DSNs are public ingest identifiers in browser code, but should
still be project-specific GlitchTip values and supplied per deployment.

The production and release image workflows read `SENTRY_URL`, `SENTRY_ORG`, and
`SENTRY_PROJECT` from GitHub repository variables, and `SENTRY_AUTH_TOKEN` from
the repository secret with the same name. For a local BuildKit image build,
pass the token as a secret mount (`--secret id=SENTRY_AUTH_TOKEN,env=SENTRY_AUTH_TOKEN`),
never as a Docker build argument. If any upload value is missing, source-map
upload is skipped and the image build remains usable.

Set `VIKUNJA_DATABASE_SSLMODE` according to the PostgreSQL service. For an
internal Coolify network, use the value supported by that resource rather than
assuming that the public database URL and the internal URL have the same TLS
requirements. If the frontend and API are deployed on different origins,
enable CORS and configure an exact HTTPS origin before starting the service.

The service secret must remain stable across restarts. If it is omitted, the
application generates a new secret at startup and previously issued JWTs stop
working after a restart.

If the deployment platform exposes secrets as files, the password can instead
be supplied with `VIKUNJA_AUTH_SINGLEUSER_PASSWORD_FILE=/run/secrets/...`.
The file value takes precedence over the direct value; trim the secret file to
the password itself.

## Where the seed credentials come from

The seed account is read at API startup from these configuration keys:

| Configuration key | Environment variable |
| --- | --- |
| `auth.singleuser.username` | `VIKUNJA_AUTH_SINGLEUSER_USERNAME` |
| `auth.singleuser.password` | `VIKUNJA_AUTH_SINGLEUSER_PASSWORD` |
| `auth.singleuser.email` | `VIKUNJA_AUTH_SINGLEUSER_EMAIL` |
| `auth.singleuser.name` | `VIKUNJA_AUTH_SINGLEUSER_NAME` |

The implementation is in [`pkg/initialize/single_user.go`](../pkg/initialize/single_user.go).
When single-user mode is enabled, the API creates the local account if it does
not exist, grants it administrator access, and synchronizes its profile and
password on startup. The password is stored as a bcrypt hash; the plaintext
password is not logged. Changing the configured password invalidates that
account's existing sessions.

The API does not load `.env` files automatically. A local `.env` file must be
exported by the shell, while Coolify values belong in its runtime environment
variable/secret store. Do not use the repository's local test credentials in
the beta. If the password is changed in the UI, update the Coolify variable as
well or the next restart will restore the configured seed password.

The username, password, and email are required. The password must be no more
than 72 bytes, local authentication must be enabled, and LDAP/OpenID must be
disabled for this mode.

## Sentry environment variable names

This repository's environment parser strips `VIKUNJA_`, lowercases the name,
and splits only on underscores. It does not infer word boundaries inside an
existing configuration key. Therefore the legacy compound keys intentionally
use `FRONTENDENABLED` and `FRONTENDDSN`, not `FRONTEND_ENABLED` or
`FRONTEND_DSN`.

| Configuration key | Environment variable |
| --- | --- |
| `sentry.enabled` | `VIKUNJA_SENTRY_ENABLED` |
| `sentry.dsn` | `VIKUNJA_SENTRY_DSN` |
| `sentry.frontendenabled` | `VIKUNJA_SENTRY_FRONTENDENABLED` |
| `sentry.frontenddsn` | `VIKUNJA_SENTRY_FRONTENDDSN` |
| `sentry.environment` | `VIKUNJA_SENTRY_ENVIRONMENT` |
| `sentry.tracesamplerate` | `VIKUNJA_SENTRY_TRACESAMPLERATE` |
| `sentry.frontendtracesamplerate` | `VIKUNJA_SENTRY_FRONTENDTRACESAMPLERATE` |
| `sentry.frontendreplaysessionsamplerate` | `VIKUNJA_SENTRY_FRONTENDREPLAYSESSIONSAMPLERATE` |
| `sentry.frontendreplaysonerrorsamplerate` | `VIKUNJA_SENTRY_FRONTENDREPLAYSONERRORSAMPLERATE` |

Safe defaults are disabled tracking, empty DSNs/environment, 0.05 trace
sampling, and 0 replay sampling. Credentials and deployment-specific DSNs do
not belong in committed config files.

## First-start checklist

- Confirm the image is healthy and `GET /api/v2/health` returns `200`.
- Confirm `GET /api/v2/info` returns `200` without authentication.
- Log in with the values stored in the Coolify runtime secrets.
- Create, update, complete, and delete a task; verify the delegation search and
  delegation create/delete flow; open list, table, Kanban, and Gantt views.
- Confirm the reverse proxy passes WebSocket upgrades.
- Take and verify a PostgreSQL backup before the first real beta data is added.
- Record the deployed immutable image tag and database backup location.
