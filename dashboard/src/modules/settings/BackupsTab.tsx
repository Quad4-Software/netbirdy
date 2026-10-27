import Breadcrumbs from "@components/Breadcrumbs";
import Button from "@components/Button";
import { Callout } from "@components/Callout";
import HelpText from "@components/HelpText";
import InlineLink from "@components/InlineLink";
import { Input } from "@components/Input";
import { Label } from "@components/Label";
import { notify } from "@components/Notification";
import Paragraph from "@components/Paragraph";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@components/table/Table";
import { useHasChanges } from "@hooks/useHasChanges";
import * as Tabs from "@radix-ui/react-tabs";
import useFetchApi, { ErrorResponse, useApiCall } from "@utils/api";
import { formatBytes } from "@utils/helpers";
import dayjs from "dayjs";
import {
  DatabaseBackupIcon,
  ExternalLinkIcon,
  RefreshCwIcon,
} from "lucide-react";
import React, { useMemo, useState } from "react";
import Skeleton from "react-loading-skeleton";
import SettingsIcon from "@/assets/icons/SettingsIcon";
import type {
  ServerBackup,
  ServerBackupRunResult,
  ServerBackupSnapshot,
  ServerBackupSnapshots,
} from "@/interfaces/ServerBackup";
import {
  backupConfigToForm,
  BackupFormState,
  backupFormValues,
  buildBackupPayload,
  describeSchedule,
  formatBackupTime,
  isValidBackupInterval,
} from "@/modules/settings/backups";

const BACKUP_DOCS_URL =
  "https://github.com/Sudo-Ivan/netbirdy/blob/main/docs/backups.md";

export default function BackupsTab() {
  const {
    data: backup,
    isLoading,
    mutate: refreshBackup,
  } = useFetchApi<ServerBackup>("/server/backup", true);
  const {
    data: snapshotData,
    isLoading: snapshotsLoading,
    mutate: refreshSnapshots,
  } = useFetchApi<ServerBackupSnapshots>("/server/backup/snapshots", true);

  const snapshots = snapshotData?.snapshots ?? [];

  return (
    <Tabs.Content value={"backups"}>
      <div className={"p-default py-6 max-w-3xl"}>
        <Breadcrumbs>
          <Breadcrumbs.Item
            href={"/settings"}
            label={"Settings"}
            icon={<SettingsIcon size={13} />}
          />
          <Breadcrumbs.Item
            href={"/settings?tab=backups"}
            label={"Backups"}
            icon={<DatabaseBackupIcon size={14} />}
            active
          />
        </Breadcrumbs>
        <h1>Backups</h1>
        <Paragraph className={"block"}>
          Back up the management data store to S3-compatible object storage on
          a schedule or on demand.{" "}
          <InlineLink href={BACKUP_DOCS_URL} target={"_blank"}>
            Learn more
            <ExternalLinkIcon size={12} />
          </InlineLink>
        </Paragraph>
        {isLoading && !backup ? (
          <div className={"mt-6"}>
            <Skeleton height={42} count={3} className={"mb-3"} />
          </div>
        ) : backup ? (
          <BackupsForm
            key={JSON.stringify(backup.config)}
            backup={backup}
            snapshots={snapshots}
            snapshotsLoading={snapshotsLoading}
            refresh={() => {
              void refreshBackup();
              void refreshSnapshots();
            }}
            refreshSnapshots={() => void refreshSnapshots()}
          />
        ) : (
          <Callout className={"mt-6"} variant={"error"}>
            The backup configuration could not be loaded from this server.
          </Callout>
        )}
      </div>
    </Tabs.Content>
  );
}

type FormProps = {
  backup: ServerBackup;
  snapshots: ServerBackupSnapshot[];
  snapshotsLoading: boolean;
  refresh: () => void;
  refreshSnapshots: () => void;
};

function BackupsForm({
  backup,
  snapshots,
  snapshotsLoading,
  refresh,
  refreshSnapshots,
}: Readonly<FormProps>) {
  const saveRequest = useApiCall<ServerBackup>("/server/backup", true);
  const runRequest = useApiCall<ServerBackupRunResult>(
    "/server/backup/run",
    true,
  );
  const [running, setRunning] = useState(false);

  const [form, setForm] = useState<BackupFormState>(() =>
    backupConfigToForm(backup.config),
  );
  const setField =
    (field: keyof BackupFormState) =>
    (e: React.ChangeEvent<HTMLInputElement>) =>
      setForm((f) => ({ ...f, [field]: e.target.value }));

  const { hasChanges, updateRef } = useHasChanges(backupFormValues(form));

  const intervalError = useMemo(() => {
    if (isValidBackupInterval(form.interval)) return "";
    return 'Use a Go duration like "30m", "6h" or "24h". Empty or "0" runs backups manually only.';
  }, [form.interval]);

  // Secrets come back masked; a non-empty stored value means the field can
  // stay blank to keep it. The bucket is the feature switch on the server.
  const stored = backup.config;
  const accessKeyStored = Boolean(stored?.s3_access_key);
  const secretKeyStored = Boolean(stored?.s3_secret_key);
  const bucketConfigured = Boolean(stored?.s3_bucket);
  const encryptionMissing =
    !stored?.encryption_key && form.encryption_key.trim() === "";

  const save = () => {
    notify({
      title: "Backups",
      description: "Backup settings were updated.",
      loadingMessage: "Saving backup settings...",
      promise: saveRequest.put(buildBackupPayload(form)).then(() => {
        const cleared = {
          ...form,
          s3_access_key: "",
          s3_secret_key: "",
          encryption_key: "",
        };
        setForm(cleared);
        updateRef(backupFormValues(cleared));
        refresh();
      }),
    });
  };

  const runNow = async () => {
    setRunning(true);
    try {
      const res = await runRequest.post({});
      notify({
        title: "Backup complete",
        description: res.key
          ? `Snapshot ${res.key} uploaded${
              res.size ? ` (${formatBytes(res.size, 1)})` : ""
            }.`
          : "The snapshot was uploaded.",
      });
      refresh();
    } catch (err) {
      notify({
        title: "Backup failed",
        description: "The backup could not be completed.",
        promise: Promise.reject(err as ErrorResponse),
      });
    } finally {
      setRunning(false);
    }
  };

  const status = backup.status;
  const runInProgress = running || Boolean(status?.running);

  return (
    <>
      <section className={"mt-8"} aria-labelledby={"backups-s3"}>
        <div className={"flex items-start justify-between"}>
          <div>
            <h2 id={"backups-s3"} className={"text-lg"}>
              S3 destination
            </h2>
            <HelpText>
              Uploads go to this bucket. Works with AWS S3 and compatible
              services such as MinIO.
            </HelpText>
          </div>
          <Button
            variant={"primary"}
            disabled={!hasChanges || intervalError !== ""}
            onClick={save}
            data-testid={"save-backup-settings"}
          >
            Save Changes
          </Button>
        </div>
        <div className={"mt-3 grid grid-cols-1 gap-4 sm:grid-cols-2"}>
          <div className={"sm:col-span-2"}>
            <Label>Endpoint</Label>
            <HelpText>
              Endpoint as host:port or http(s) URL. Leave empty to use the AWS
              endpoint resolved from the region.
            </HelpText>
            <Input
              placeholder={"minio.internal:9000"}
              value={form.s3_endpoint}
              onChange={setField("s3_endpoint")}
              data-testid={"backup-endpoint-input"}
            />
          </div>
          <div>
            <Label>Region</Label>
            <Input
              placeholder={"eu-central-1"}
              value={form.s3_region}
              onChange={setField("s3_region")}
              data-testid={"backup-region-input"}
            />
          </div>
          <div>
            <Label>Bucket</Label>
            <HelpText>The destination bucket. Empty disables backups.</HelpText>
            <Input
              placeholder={"netbirdy-backups"}
              value={form.s3_bucket}
              onChange={setField("s3_bucket")}
              data-testid={"backup-bucket-input"}
            />
          </div>
          <div>
            <Label>Access key</Label>
            <Input
              type={"password"}
              showPasswordToggle
              placeholder={
                accessKeyStored ? "Leave blank to keep current" : "Access key"
              }
              value={form.s3_access_key}
              onChange={setField("s3_access_key")}
              data-testid={"backup-access-key-input"}
            />
            {accessKeyStored && (
              <HelpText margin={false} className={"mt-1.5"}>
                A value is stored. Leave blank to keep it.
              </HelpText>
            )}
          </div>
          <div>
            <Label>Secret key</Label>
            <Input
              type={"password"}
              showPasswordToggle
              placeholder={
                secretKeyStored ? "Leave blank to keep current" : "Secret key"
              }
              value={form.s3_secret_key}
              onChange={setField("s3_secret_key")}
              data-testid={"backup-secret-key-input"}
            />
            {secretKeyStored && (
              <HelpText margin={false} className={"mt-1.5"}>
                A value is stored. Leave blank to keep it.
              </HelpText>
            )}
          </div>
          <div className={"sm:col-span-2"}>
            <Label>Prefix</Label>
            <HelpText>
              Optional path prefix for snapshot objects inside the bucket.
            </HelpText>
            <Input
              placeholder={"mgmt/"}
              value={form.s3_prefix}
              onChange={setField("s3_prefix")}
              data-testid={"backup-prefix-input"}
            />
          </div>
        </div>
      </section>

      <section className={"mt-10"} aria-labelledby={"backups-schedule"}>
        <h2 id={"backups-schedule"} className={"text-lg"}>
          Schedule and encryption
        </h2>
        <div className={"mt-3 grid grid-cols-1 gap-4 sm:grid-cols-2"}>
          <div>
            <Label>Interval</Label>
            <HelpText>
              {
                'How often backups run, for example "6h" or "24h". Empty or "0" runs backups only with Backup now.'
              }
            </HelpText>
            <Input
              placeholder={"6h"}
              error={intervalError}
              value={form.interval}
              onChange={setField("interval")}
              data-testid={"backup-interval-input"}
            />
          </div>
          <div>
            <Label>Encryption passphrase</Label>
            <HelpText>
              Snapshots are encrypted with this passphrase before upload.
              {stored?.encryption_key
                ? " A passphrase is stored. Leave blank to keep it."
                : ""}
            </HelpText>
            <Input
              type={"password"}
              showPasswordToggle
              placeholder={
                stored?.encryption_key
                  ? "Leave blank to keep current"
                  : "Passphrase"
              }
              value={form.encryption_key}
              onChange={setField("encryption_key")}
              data-testid={"backup-encryption-key-input"}
            />
          </div>
        </div>
        {encryptionMissing && (
          <Callout className={"mt-4"} variant={"warning"}>
            Set an encryption passphrase. Without it, snapshots cannot be
            uploaded and backups refuse to run.
          </Callout>
        )}
      </section>

      <section className={"mt-10"} aria-labelledby={"backups-snapshots"}>
        <div className={"flex items-center justify-between"}>
          <h2 id={"backups-snapshots"} className={"text-lg"}>
            Snapshots
          </h2>
          <div className={"flex items-center gap-3"}>
            <Button
              variant={"secondary"}
              size={"sm"}
              onClick={refreshSnapshots}
              disabled={snapshotsLoading}
              data-testid={"backup-snapshots-refresh"}
            >
              <RefreshCwIcon size={14} />
              Refresh
            </Button>
            <Button
              variant={"primary"}
              size={"sm"}
              onClick={runNow}
              disabled={runInProgress || !bucketConfigured}
              data-testid={"backup-run-now"}
            >
              <DatabaseBackupIcon size={14} />
              {runInProgress ? "Running..." : "Backup now"}
            </Button>
          </div>
        </div>
        {!bucketConfigured && (
          <HelpText className={"mt-2"}>
            Save a destination bucket above before running a backup.
          </HelpText>
        )}
        <div className={"mt-4 flex flex-wrap items-center gap-x-8 gap-y-3"}>
          <Stat
            label={"Status"}
            value={
              status?.running
                ? "Running now"
                : status?.enabled
                ? "Enabled"
                : "Disabled"
            }
          />
          <Stat label={"Schedule"} value={describeSchedule(stored?.interval)} />
          <Stat label={"Last run"} value={formatBackupTime(status?.last_run)} />
          <Stat label={"Next run"} value={formatBackupTime(status?.next_run)} />
        </div>
        {status?.last_snapshot && (
          <div className={"mt-3"}>
            <Stat label={"Last snapshot"} value={status.last_snapshot} />
          </div>
        )}
        {status?.last_error && (
          <Callout className={"mt-4"} variant={"error"}>
            {status.last_error}
          </Callout>
        )}
        {snapshots.length === 0 ? (
          <div className={"mt-4 text-sm text-nb-gray-400"}>
            No snapshots yet.
          </div>
        ) : (
          <div className={"mt-4"}>
            <Table minimal>
              <TableHeader minimal>
                <TableRow minimal>
                  <TableHead minimal>Key</TableHead>
                  <TableHead minimal>Size</TableHead>
                  <TableHead minimal>Date</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {snapshots.map((s) => (
                  <TableRow minimal key={s.key}>
                    <TableCell minimal className={"font-mono text-xs"}>
                      {s.key}
                    </TableCell>
                    <TableCell minimal>{formatBytes(s.size, 1)}</TableCell>
                    <TableCell minimal>
                      {dayjs(s.last_modified).format("MMM D, YYYY [at] h:mm A")}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </section>

      <section className={"mt-10"} aria-labelledby={"backups-restore"}>
        <h2 id={"backups-restore"} className={"text-lg"}>
          Restore
        </h2>
        <Paragraph className={"block mt-2"}>
          Restoring happens offline: stop the management server, then run{" "}
          <span className={"font-mono"}>netbirdy-mgmt backup restore</span>.
          See the{" "}
          <InlineLink href={BACKUP_DOCS_URL} target={"_blank"}>
            backup documentation
            <ExternalLinkIcon size={12} />
          </InlineLink>
          .
        </Paragraph>
      </section>
    </>
  );
}

const Stat = ({ label, value }: { label: string; value: string }) => (
  <div>
    <div className={"text-xs text-nb-gray-400"}>{label}</div>
    <div className={"text-sm break-all"}>{value}</div>
  </div>
);
