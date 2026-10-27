# Encrypted S3 backups

The management server can snapshot its store on a schedule or on demand,
encrypt the snapshot client-side with age passphrase encryption, and upload it
to an S3-compatible bucket (AWS S3, MinIO, Backblaze B2, Cloudflare R2).

Supported store engine: SQLite only. Servers running Postgres (or any other
engine) get a clear "not supported" error on every run instead of a fake
success; back those stores up with the engine's own tooling (pg_dump, WAL-G).

## How it works

1. The SQLite store is snapshot with `VACUUM INTO`, producing one consistent
   database file while the server keeps running. The `-wal` and `-shm`
   sidecars do not need copying.
2. The database file plus a small `manifest.json` (schema version, timestamp,
   netbirdy version, store engine) are streamed into a `.tar.gz` archive.
   Plaintext never exists outside the server process.
3. The archive is encrypted with age using the scrypt passphrase from
   `NB_BACKUP_ENCRYPTION_KEY` and uploaded as
   `<prefix>netbirdy-backup-<UTC timestamp>.tar.gz.age`.

Backups refuse to run without an encryption key: the server never uploads a
plaintext snapshot.

## Configuration

Everything is optional. The feature is off until a bucket is set.

| Env var                    | API field        | Description                                        |
| -------------------------- | ---------------- | -------------------------------------------------- |
| `NB_BACKUP_S3_ENDPOINT`    | `s3_endpoint`    | Endpoint `host[:port]` or `http(s)://` URL. Empty resolves the AWS endpoint from the region. |
| `NB_BACKUP_S3_REGION`      | `s3_region`      | Signing region, e.g. `us-east-1`.                   |
| `NB_BACKUP_S3_BUCKET`      | `s3_bucket`      | Destination bucket. Empty disables the feature.     |
| `NB_BACKUP_S3_ACCESS_KEY`  | `s3_access_key`  | Access key.                                        |
| `NB_BACKUP_S3_SECRET_KEY`  | `s3_secret_key`  | Secret key.                                        |
| `NB_BACKUP_S3_PREFIX`      | `s3_prefix`      | Object key prefix, default `netbirdy-backups/`.     |
| `NB_BACKUP_INTERVAL`       | `interval`       | Go duration between scheduled runs, e.g. `24h`. Empty or `0` means manual runs only. |
| `NB_BACKUP_ENCRYPTION_KEY` | `encryption_key` | age passphrase. Required before any run succeeds.   |

Custom endpoints (MinIO, B2, R2) get path-style requests automatically; AWS
endpoints keep virtual-host style. Use `http://` in the endpoint for
plain-text object storage (local MinIO only, never for real data over a
network).

### Precedence

1. `PUT /api/server/backup` writes the full config to `backup.json` inside the
   datadir (next to `store.db`), owner-only permissions, and persists across
   restarts.
2. When `backup.json` exists it wins over the environment wholesale.
3. Otherwise the `NB_BACKUP_*` env vars apply.

To go back to env-based config, delete `backup.json` while the server is
stopped.

## Dashboard

The dashboard Settings page exposes a Backups tab that talks to the REST API
below: it shows the masked config and scheduler status, edits the same fields
and triggers manual runs. Secret fields left blank keep the stored values.

## REST API

All endpoints require an admin or owner role.

| Endpoint                            | Description                                        |
| ----------------------------------- | -------------------------------------------------- |
| `GET /api/server/backup`            | Config with secrets masked as `********` + status. |
| `PUT /api/server/backup`            | Update config. Empty or masked secret fields keep the stored value. |
| `POST /api/server/backup/run`       | Run a backup now, returns the object key and size.  |
| `GET /api/server/backup/snapshots`  | List stored snapshots (key, size, last modified).   |

Example:

```sh
curl -X PUT https://mgmt.example.com/api/server/backup \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "s3_endpoint": "minio.internal:9000",
    "s3_region": "us-east-1",
    "s3_bucket": "netbirdy-backups",
    "s3_access_key": "AKIA...",
    "s3_secret_key": "...",
    "s3_prefix": "prod/netbirdy/",
    "interval": "24h",
    "encryption_key": "a long random passphrase"
  }'
```

`GET` status fields: `enabled`, `running`, `last_run`, `last_error`,
`last_snapshot`, `next_run` (RFC 3339 strings, empty when unset).

## Manual run

```sh
curl -X POST https://mgmt.example.com/api/server/backup/run \
  -H "Authorization: Bearer $TOKEN"
```

## Restore

Restore is offline: it must run while the management server is STOPPED.
Restoring over a live `store.db` corrupts the running server.

```sh
systemctl stop netbirdy-mgmt   # or whatever runs the binary

netbirdy-mgmt backup list \
  --endpoint minio.internal:9000 --bucket netbirdy-backups \
  --access-key ... --secret-key ...

netbirdy-mgmt backup restore --key latest \
  --encryption-key "a long random passphrase" \
  --datadir /var/lib/netbird

systemctl start netbirdy-mgmt
```

`restore` downloads the object, decrypts it, and writes the store file to
`--output` (default: the configured sqlite store path under `--datadir`,
honoring `NB_STORE_ENGINE_SQLITE_FILE`). An existing destination file is kept
as `<file>.pre-restore-<timestamp>` so a bad restore is reversible.

All S3 flags fall back to the `NB_BACKUP_*` env vars and to `backup.json` in
`--datadir`, so on the server itself most flags can be omitted.

## Encryption notes

- Snapshots are age-encrypted with a passphrase-derived scrypt recipient
  (interactive work factor). The same passphrase decrypts any snapshot: store
  it somewhere safe and independent of the bucket, because losing it makes
  every backup useless.
- The bucket only ever sees ciphertext; the passphrase, S3 credentials and
  API responses never expose it (responses return `********`).
- Rotating the passphrase only affects future snapshots; keep old passphrases
  if you keep old snapshots.

## Limitations

- SQLite store engine only.
- No retention management: objects are never deleted; lifecycle rules on the
  bucket are the way to expire old snapshots.
- Scheduled runs are serialized: a run longer than the interval pushes the
  next one back rather than overlapping.
