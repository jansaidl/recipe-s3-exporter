#!/usr/bin/env bash
#
# zerops-backup-to-s3.sh
# ----------------------
# Fetches the backup list for a Zerops service from the Zerops API, picks the
# latest backup, and streams it straight to an S3 storage (no local file).
#
# Auth: Zerops access / integration token (Bearer).
# Endpoints verified against the zeropsio/zerops-go SDK.
#
# Usage (everything via environment variables):
#   export ZEROPS_TOKEN="..."           # required
#   export SERVICE_STACK_ID="..."       # required
#   export S3_BUCKET="my-backups"       # required
#   ./zerops-backup-to-s3.sh
#
set -euo pipefail

# ─── Configuration ───────────────────────────────────────────────────────────
: "${ZEROPS_TOKEN:?Set ZEROPS_TOKEN (Zerops access / integration token)}"
: "${SERVICE_STACK_ID:?Set SERVICE_STACK_ID (target service)}"
: "${S3_BUCKET:?Set S3_BUCKET (target bucket)}"

ZEROPS_API="${ZEROPS_API:-https://api.app-prg1.zerops.io/api/rest/public}"

# S3 target
S3_PREFIX="${S3_PREFIX:-zerops-backups}"
S3_ENDPOINT="${S3_ENDPOINT:-}"          # optional: non-AWS S3 (MinIO, Zerops Object Storage…)
UPLOADER="${UPLOADER:-aws}"             # aws | rclone
STREAM="${STREAM:-1}"                   # 1 = stream straight to S3, 0 = download to temp file first

# ─── Helpers ─────────────────────────────────────────────────────────────────
log() { printf '\033[1;34m[%(%H:%M:%S)T]\033[0m %s\n' -1 "$*" >&2; }
die() { printf '\033[1;31m[error]\033[0m %s\n' "$*" >&2; exit 1; }

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

# Dependency checks
for bin in curl jq; do
  command -v "$bin" >/dev/null || die "Missing tool '$bin'."
done
command -v "$UPLOADER" >/dev/null || die "Missing '$UPLOADER' for the S3 upload."

# Zerops API call:  api METHOD PATH [extra curl args...]
api() {
  local method="$1" path="$2"; shift 2
  curl -fsS -X "$method" \
    -H "Authorization: Bearer ${ZEROPS_TOKEN}" \
    -H "Accept: application/json" \
    "$@" \
    "${ZEROPS_API}${path}"
}

log "Service stack ID: ${SERVICE_STACK_ID}"

# ─── 1) Backup list ──────────────────────────────────────────────────────────
log "Fetching backup list…"
BACKUPS_JSON="$(api GET "/service-stack/${SERVICE_STACK_ID}/backup")"

COUNT="$(jq '.files | length' <<<"$BACKUPS_JSON")"
[[ "${COUNT:-0}" -gt 0 ]] || die "No backups found."
log "Backups found: ${COUNT}"

# Latest backup = highest 'name' (Zerops names backups by date -> sortable).
# If 'name' isn't date-sortable, switch to sort_by(.metadata.created) etc.
LATEST="$(jq -c '.files | sort_by(.name) | last' <<<"$BACKUPS_JSON")"
BACKUP_NAME="$(jq -r '.name' <<<"$LATEST")"
BACKUP_SIZE="$(jq -r '.size // 0' <<<"$LATEST")"
log "Latest backup: ${BACKUP_NAME} (${BACKUP_SIZE} B)"

# ─── 2) Get the download URL ─────────────────────────────────────────────────
log "Requesting download URL…"
# The {date} path param maps to the backup identifier = the 'name' field.
# (If this returns 404, try .path instead of .name from the list above.)
ENC_NAME="$(jq -rn --arg s "$BACKUP_NAME" '$s|@uri')"
DOWNLOAD_URL="$(
  api POST "/service-stack/${SERVICE_STACK_ID}/backup/download-url/${ENC_NAME}" \
      -H "Content-Type: application/json" -d '{}' \
    | jq -r '.url // empty'
)"
[[ -n "$DOWNLOAD_URL" ]] || die "Failed to obtain the download URL."

# ─── 3) Stream (or download) and upload to S3 ────────────────────────────────
S3_KEY="${S3_PREFIX}/${SERVICE_STACK_ID}/${BACKUP_NAME}"
S3_DEST="s3://${S3_BUCKET}/${S3_KEY}"

# Reads the backup bytes from stdin and uploads them to S3.
upload_from_stdin() {
  case "$UPLOADER" in
    aws)
      local args=(s3 cp - "$S3_DEST")
      [[ -n "$S3_ENDPOINT" ]] && args+=(--endpoint-url "$S3_ENDPOINT")
      # Help the CLI size multipart parts correctly for large streams.
      [[ "${BACKUP_SIZE:-0}" -gt 0 ]] && args+=(--expected-size "$BACKUP_SIZE")
      aws "${args[@]}"
      ;;
    rclone)
      : "${RCLONE_REMOTE:?Set RCLONE_REMOTE (name of the configured rclone remote)}"
      local rargs=(rcat "${RCLONE_REMOTE}:${S3_BUCKET}/${S3_KEY}")
      [[ "${BACKUP_SIZE:-0}" -gt 0 ]] && rargs+=(--size "$BACKUP_SIZE")
      rclone "${rargs[@]}"
      ;;
    *)
      die "Unknown UPLOADER '${UPLOADER}' (supported: aws, rclone)."
      ;;
  esac
}

if [[ "$STREAM" == "1" ]]; then
  log "Streaming backup directly to ${S3_DEST}…"
  # pipefail makes the whole pipeline fail if the download fails.
  curl -fSL "$DOWNLOAD_URL" | upload_from_stdin
else
  LOCAL_FILE="${WORKDIR}/${BACKUP_NAME}"
  log "Downloading backup to a temp file…"
  curl -fSL --progress-bar -o "$LOCAL_FILE" "$DOWNLOAD_URL"
  log "Downloaded: $(du -h "$LOCAL_FILE" | cut -f1). Uploading to ${S3_DEST}…"
  upload_from_stdin < "$LOCAL_FILE"
fi

log "Done ✅  Backup uploaded: ${S3_DEST}"
