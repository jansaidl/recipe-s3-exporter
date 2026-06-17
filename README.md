# Zerops Backup → S3 Exporter

A web dashboard that exports [Zerops](https://zerops.io) managed-service backups
(PostgreSQL, MariaDB, KeyDB, shared storage, …) to any **S3-compatible** object
storage, on a schedule, with live progress — built on the flow proven by
[`export.sh`](./export.sh).

Single Go binary: server-rendered UI (templ + HTMX + Tailwind), PostgreSQL for
state, a worker pool for transfers, and a cron scheduler. **All secrets
(Zerops tokens, S3 credentials) are encrypted at rest** with AES-256-GCM using a
master key supplied via the environment.

## Features

- **Authentication** — multi-user with roles (`admin` / `viewer`), bcrypt
  passwords, Postgres-backed sessions. First admin bootstrapped from env.
- **Zerops tokens** — store and validate integration tokens; used to read
  backups via the Zerops API.
- **Export targets** — any S3-compatible endpoint (AWS S3, MinIO, Zerops Object
  Storage, …); credentials encrypted; one-click connection test.
- **Export jobs** — pick token → project → service (cascading, live from the
  Zerops API), filter by **backup tags**, choose a target and a cron schedule.
- **Workers & progress** — concurrent streaming download→upload with a live
  progress bar (HTMX polling); idempotent (skips already-exported backups).
- **Explore** — browse exported objects per target with presigned downloads.
- **Encryption** — every secret encrypted with `MASTER_KEY`; the app refuses to
  start without a valid key.

## How an export works

1. Resolve the job's Zerops token and list the service's backups.
2. Pick the **latest** backup carrying **all** of the job's tag filter
   (empty filter = latest of any).
3. Skip if the object already exists at the target (idempotent).
4. Request a temporary download URL, stream the bytes straight into S3 while
   recording progress, and mark the run success/failed/skipped.

## Run locally

Requires Go 1.24+, a running PostgreSQL instance, and (for styling) the Tailwind CLI.

```bash
# 1. Configure (point DATABASE_URL at your PostgreSQL instance)
cp .env.example .env
export $(grep -v '^#' .env | xargs)
export MASTER_KEY="$(openssl rand -base64 32)"

# 2. Generate templates + CSS, then run
templ generate
tailwindcss -c tailwind.config.js \
  -i internal/web/static/css/input.css \
  -o internal/web/static/css/app.css --minify
go run ./cmd/server
```

Open http://localhost:8080 and sign in with `ADMIN_EMAIL` / `ADMIN_PASSWORD`.

### First steps in the UI

1. **Tokens** → add a Zerops integration token (validated on save).
2. **Targets** → add an S3 target (connection tested on save).
3. **Jobs** → create a job: token → project → service → tag filter → target →
   schedule. Use **Run now** to test immediately.
4. **Dashboard / Runs** → watch live progress; **Explore** → browse results.

## Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | ✅ | — | PostgreSQL connection string |
| `MASTER_KEY` | ✅ | — | base64 of 32 bytes, or a literal 32-char string |
| `ADMIN_EMAIL` | — | — | bootstraps the first admin when no users exist |
| `ADMIN_PASSWORD` | — | — | password for the bootstrapped admin |
| `PORT` | — | `8080` | HTTP listen port |
| `EXPORT_WORKERS` | — | `2` | concurrent export workers |
| `SECURE_COOKIES` | — | `false` | mark the session cookie Secure (set `true` behind HTTPS) |
| `ZEROPS_API` | — | Zerops prod | Zerops public REST API base URL |

## Deploy to Zerops

This repository is a [Zerops recipe](https://docs.zerops.io/references/import).
The recipe metadata and import definitions live under `.zerops-recipe/`:

```
.zerops-recipe/
  README.md                     recipe page content (fragments)
  1 — Small Production/
    import.yaml                 single app container, NON_HA PostgreSQL
    README.md
  2 — HA Production/
    import.yaml                 app autoscaling 2–4, HA PostgreSQL
    README.md
zerops.yml                      build & run config for the app (buildFromGit target)
```

Deploy from the Zerops recipe page, or import an environment via the CLI:

```bash
zerops project import ".zerops-recipe/1 — Small Production/import.yaml"
```

Each environment creates three services:

- **app** — this dashboard (built via [`zerops.yml`](./zerops.yml)); `MASTER_KEY`
  and `ADMIN_PASSWORD` are generated automatically, and `ADMIN_EMAIL` is prompted
  on deploy. Update `buildFromGit` in the `import.yaml` files to point at your fork.
- **db** — PostgreSQL 16 (NON_HA or HA per environment); `DATABASE_URL` is wired
  from the `${db_*}` env vars in `zerops.yml`.
- **storage** — a Zerops Object Storage bucket you can use as an export target,
  via `${storage_apiUrl}`, `${storage_accessKeyId}`, `${storage_secretAccessKey}`,
  `${storage_bucketName}`.

After deploy, read the generated `ADMIN_PASSWORD` from the app service's
environment variables and sign in via its subdomain.

## Architecture

```
cmd/server            entrypoint: config, migrations, workers, scheduler, HTTP
internal/config       env configuration + MASTER_KEY validation
internal/crypto       AES-256-GCM encrypt/decrypt for secrets
internal/db           pgx pool, embedded migrations, repositories
internal/models       domain types
internal/zerops       Zerops API client (projects, services, backups, downloads)
internal/storage      S3-compatible client (minio-go): upload, list, presign
internal/worker       worker pool + counting-reader progress + run executor
internal/scheduler    robfig/cron scheduling of enabled jobs
internal/auth         bcrypt, scs sessions, auth/role middleware
internal/web          templ templates, HTMX handlers, router
```

## Security notes

- Secrets are stored as AES-256-GCM ciphertext; the master key lives only in the
  environment and is never persisted.
- Rotating `MASTER_KEY` invalidates existing ciphertext — re-enter tokens and
  target credentials after a rotation.
- Run behind HTTPS in production and set `SECURE_COOKIES=true`.
