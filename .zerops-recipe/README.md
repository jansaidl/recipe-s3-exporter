<!-- #ZEROPS_EXTRACT_START:name# -->
Zerops S3 Backup Exporter
<!-- #ZEROPS_EXTRACT_END:name# -->

<!-- #ZEROPS_EXTRACT_START:shape# -->
app
<!-- #ZEROPS_EXTRACT_END:shape# -->

<!-- #ZEROPS_EXTRACT_START:intro# -->
A dashboard that exports your Zerops managed-service backups to any S3-compatible
object storage — on a schedule, filtered by backup tags, with live progress and
every secret encrypted at rest.
<!-- #ZEROPS_EXTRACT_END:intro# -->

<!-- #ZEROPS_EXTRACT_START:description# -->
The Zerops S3 Backup Exporter turns your scheduled Zerops backups into durable,
off-platform copies. Connect a Zerops integration token, point it at any
S3-compatible bucket (AWS S3, MinIO, Zerops Object Storage, …), and define export
jobs that pick the latest backup of a service — optionally filtered by tags such
as `daily` or `weekly` — and stream it straight into your bucket. A live
dashboard shows running transfers, full run history, and lets you browse and
download what has already been exported.

It runs as a single Go binary with a server-rendered UI (templ + HTMX), backed by
PostgreSQL for users, tokens, targets, jobs and run history. All sensitive values
(Zerops tokens and S3 credentials) are encrypted with AES-256-GCM using a master
key held only in the environment.

This recipe deploys the application, a PostgreSQL database, and a ready-to-use
Zerops Object Storage bucket. Choose **Small Production** for a cost-efficient
single-container setup, or **HA Production** for a highly available, horizontally
scaled deployment with HA PostgreSQL.
<!-- #ZEROPS_EXTRACT_END:description# -->

<!-- #ZEROPS_EXTRACT_START:features# -->
- Schedule backup exports with cron and run them on demand
- Select backups by Zerops tags (e.g. `daily`, `weekly`, `monthly`)
- Stream to any S3-compatible target (AWS S3, MinIO, Zerops Object Storage)
- Live progress for running exports and complete run history
- Browse and download exported objects with presigned links
- Multi-user access with admin / viewer roles
- All secrets encrypted at rest with an environment-supplied master key
- Idempotent exports that skip backups already copied
<!-- #ZEROPS_EXTRACT_END:features# -->

<!-- #ZEROPS_EXTRACT_START:takeover-guide# -->
After the project deploys:

1. Open the **app** service's subdomain URL.
2. Sign in with the `ADMIN_EMAIL` you provided and the generated `ADMIN_PASSWORD`
   (read it from the app service's environment variables in the Zerops GUI).
3. Go to **Tokens** and add a Zerops integration token (created under your account
   settings in Zerops). It is validated against the API and stored encrypted.
4. Go to **Targets** and add an S3 target. To use the bundled Zerops Object
   Storage, take `${storage_apiUrl}`, `${storage_accessKeyId}`,
   `${storage_secretAccessKey}` and `${storage_bucketName}` from the **storage**
   service's environment variables.
5. Go to **Jobs**, create an export job (token → project → service → tag filter →
   target → schedule) and use **Run now** to verify it end to end.

Rotating `MASTER_KEY` invalidates previously stored secrets — re-enter tokens and
target credentials after any rotation.
<!-- #ZEROPS_EXTRACT_END:takeover-guide# -->

<!-- #ZEROPS_EXTRACT_START:knowledge-base# -->
### Architecture

A single Go binary serves the dashboard and runs the export workers and cron
scheduler in-process:

- **web** — templ + HTMX server-rendered UI and JSON-free form handlers
- **worker** — a pool that streams each backup download straight into S3 with
  bounded-memory multipart uploads and live, persisted progress
- **scheduler** — `robfig/cron` registering every enabled job, reloaded on change
- **db** — PostgreSQL (pgx) with embedded migrations applied at startup
- **crypto** — AES-256-GCM encryption of all stored secrets

An export resolves the job's token, picks the latest backup carrying all of the
job's tags, requests a temporary download URL, and streams the bytes into the
target — skipping anything already present (idempotent).

### Environment variables

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | yes | PostgreSQL connection string (wired from the `db` service) |
| `MASTER_KEY` | yes | 32-byte key (base64 or a literal 32-char string) encrypting all secrets |
| `ADMIN_EMAIL` | — | bootstraps the first admin on first boot |
| `ADMIN_PASSWORD` | — | password for the bootstrapped admin |
| `PORT` | — | HTTP port (default `8080`) |
| `EXPORT_WORKERS` | — | concurrent export workers (default `2`) |
| `SECURE_COOKIES` | — | set `true` to mark the session cookie Secure (HTTPS) |
| `ZEROPS_API` | — | Zerops public REST API base URL |
| `ZEROPS_AUTH_SCHEME` | — | Authorization scheme for Zerops calls (default `Bearer`) |

### Troubleshooting

- **Token validation fails** — confirm it is a Zerops integration/access token
  with access to the account whose projects you want to export.
- **Export reports 0 bytes** — the backup archive may not be ready yet; the
  worker retries with a fresh download URL. Check the app logs for the
  `[zerops] <- download …` line showing the actual status and length.
- **S3 connection test fails** — verify the endpoint host, bucket, region and
  whether path-style addressing is required (most non-AWS S3 needs it).
<!-- #ZEROPS_EXTRACT_END:knowledge-base# -->
