import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import type { ServerBackupConfig } from "@/interfaces/ServerBackup";
import { isZeroTime } from "@/modules/settings/updates";

dayjs.extend(relativeTime);

// The API masks stored secrets with this value on reads. Sending the mask
// back, or an empty string, keeps the stored value.
export const MASKED_SECRET = "********";

export const isMaskedSecret = (value?: string): boolean =>
  value?.trim() === MASKED_SECRET;

// Normalizes a secret input for submission: the mask or a blank input both
// mean "keep the stored value", which the API expects as an empty string.
export const secretForSubmit = (value: string): string =>
  isMaskedSecret(value) ? "" : value.trim();

// Accepts a Go-style duration such as "30m", "6h" or "1h30m". An empty value
// or "0" is valid and means backups run manually only.
export const isValidBackupInterval = (value: string): boolean => {
  const v = value.trim();
  if (v === "" || v === "0") return true;
  return /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/.test(v);
};

export type BackupFormState = {
  s3_endpoint: string;
  s3_region: string;
  s3_bucket: string;
  s3_access_key: string;
  s3_secret_key: string;
  s3_prefix: string;
  interval: string;
  encryption_key: string;
};

// Maps a stored config to the editable form. Secrets come back masked from
// the API, so the inputs start blank: leaving them blank keeps the stored
// value, typing a new one replaces it.
export const backupConfigToForm = (
  config: ServerBackupConfig | null | undefined,
): BackupFormState => ({
  s3_endpoint: config?.s3_endpoint ?? "",
  s3_region: config?.s3_region ?? "",
  s3_bucket: config?.s3_bucket ?? "",
  s3_access_key: "",
  s3_secret_key: "",
  s3_prefix: config?.s3_prefix ?? "",
  interval: config?.interval ?? "",
  encryption_key: "",
});

// Stable value list for change tracking, ordered the same every call.
export const backupFormValues = (form: BackupFormState): string[] => [
  form.s3_endpoint,
  form.s3_region,
  form.s3_bucket,
  form.s3_prefix,
  form.interval,
  form.s3_access_key,
  form.s3_secret_key,
  form.encryption_key,
];

// The PUT body is the bare ServerBackupConfig object.
export const buildBackupPayload = (
  form: BackupFormState,
): ServerBackupConfig => ({
  s3_endpoint: form.s3_endpoint.trim(),
  s3_region: form.s3_region.trim(),
  s3_bucket: form.s3_bucket.trim(),
  s3_access_key: secretForSubmit(form.s3_access_key),
  s3_secret_key: secretForSubmit(form.s3_secret_key),
  s3_prefix: form.s3_prefix.trim(),
  interval: form.interval.trim(),
  encryption_key: secretForSubmit(form.encryption_key),
});

// The schedule shown next to the status: an interval means scheduled runs,
// anything else means manual runs only.
export const describeSchedule = (interval?: string): string =>
  interval && isValidBackupInterval(interval) && interval.trim() !== "0"
    ? `Every ${interval}`
    : "Manual only";

export const formatBackupTime = (iso?: string): string =>
  !iso || isZeroTime(iso) ? "Never" : dayjs(iso).fromNow();
