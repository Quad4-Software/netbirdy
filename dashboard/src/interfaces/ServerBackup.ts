// Types for the management server backup endpoints:
//   GET  /server/backup          -> ServerBackup
//   PUT  /server/backup          <- ServerBackupConfig, -> ServerBackup
//   POST /server/backup/run      -> ServerBackupRunResult
//   GET  /server/backup/snapshots -> ServerBackupSnapshots
// Declared here until the generated ServerBackup* types land in
// shared/management/http/api/types.gen.go. Field names match the
// ServerBackup* schemas in shared/management/http/api/openapi.yml.

export interface ServerBackupConfig {
  s3_endpoint: string;
  s3_region: string;
  s3_bucket: string;
  s3_access_key: string;
  s3_secret_key: string;
  s3_prefix: string;
  interval: string;
  encryption_key: string;
}

export interface ServerBackupStatus {
  enabled: boolean;
  running: boolean;
  last_run: string;
  last_error: string;
  last_snapshot: string;
  next_run: string;
}

export interface ServerBackup {
  config: ServerBackupConfig | null;
  status: ServerBackupStatus;
}

export interface ServerBackupRunResult {
  key: string;
  size: number;
}

export interface ServerBackupSnapshot {
  key: string;
  size: number;
  last_modified: string;
}

export interface ServerBackupSnapshots {
  snapshots: ServerBackupSnapshot[];
}
