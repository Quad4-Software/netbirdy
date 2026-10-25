import Breadcrumbs from "@components/Breadcrumbs";
import Button from "@components/Button";
import { Callout } from "@components/Callout";
import InlineLink from "@components/InlineLink";
import { notify } from "@components/Notification";
import Paragraph from "@components/Paragraph";
import * as Tabs from "@radix-ui/react-tabs";
import useFetchApi, { useApiCall, useApiCallWithStatus } from "@utils/api";
import { formatBytes } from "@utils/helpers";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import {
  ArrowUpCircleIcon,
  CheckCircle2Icon,
  RefreshCwIcon,
} from "lucide-react";
import React, { useState } from "react";
import SettingsIcon from "@/assets/icons/SettingsIcon";
import { useDialog } from "@/contexts/DialogProvider";
import {
  isNewer,
  isZeroTime,
  meterPercent,
  normalizeVersion,
  UpdatesInfo,
  waitForServerVersion,
} from "@/modules/settings/updates";

dayjs.extend(relativeTime);

const when = (iso?: string) =>
  isZeroTime(iso) ? "Never" : dayjs(iso).fromNow();

export default function UpdatesTab() {
  const { data, mutate } = useFetchApi<UpdatesInfo>("/updates", true, true);
  const reader = useApiCall<UpdatesInfo>("/updates", true);
  const checkCall = useApiCallWithStatus("/updates/check");
  const serverCall = useApiCallWithStatus("/updates/server");
  const { confirm } = useDialog();
  const [progress, setProgress] = useState<string>();
  const [result, setResult] = useState<{ ok: boolean; message: string }>();
  const [busy, setBusy] = useState(false);

  const current = data?.server_version ?? "";
  const latest = data?.latest_version ?? "";
  const updateAvailable = isNewer(latest, current);
  const updaterReady = data?.updater.available ?? false;

  const checkNow = async () => {
    const res = await checkCall.post();
    const messages: Record<number, string> = {
      202: "Checking GitHub releases now.",
      409: "The update mirror is not enabled.",
      429: "A check ran recently. Try again in a moment.",
    };
    const text = messages[res.code] ?? "The check could not be started.";
    notify({
      title: "Update check",
      description: text,
      loadingMessage: "Starting update check...",
      promise:
        res.code === 202 ? Promise.resolve() : Promise.reject(new Error(text)),
    });
    setTimeout(() => mutate(), 3000);
  };

  const updateServer = async () => {
    const ok = await confirm({
      title: `Update server to ${normalizeVersion(latest)}`,
      description:
        "The updater takes a backup of the data volume, restarts the server, and rolls back if it does not become healthy. The dashboard is unavailable for a short time.",
      confirmText: "Update server",
      cancelText: "Cancel",
      type: "warning",
    });
    if (!ok) return;
    setBusy(true);
    setResult(undefined);
    const res = await serverCall.post({ version: `v${normalizeVersion(latest)}` });
    if (res.code !== 202) {
      setBusy(false);
      setResult({
        ok: false,
        message:
          res.code === 501
            ? "No updater is configured for this server."
            : "The updater could not start the update.",
      });
      return;
    }
    const outcome = await waitForServerVersion({
      fetchInfo: () => reader.get(),
      target: latest,
      sleep: (ms) => new Promise((r) => setTimeout(r, ms)),
      now: Date.now,
      onProgress: setProgress,
    });
    setBusy(false);
    setProgress(undefined);
    setResult({ ok: outcome.phase === "success", message: outcome.message });
    mutate();
  };

  return (
    <Tabs.Content value={"updates"}>
      <div className={"p-default py-6 max-w-3xl"}>
        <Breadcrumbs>
          <Breadcrumbs.Item
            href={"/settings"}
            label={"Settings"}
            icon={<SettingsIcon size={13} />}
          />
          <Breadcrumbs.Item
            href={"/settings?tab=updates"}
            label={"Updates"}
            icon={<ArrowUpCircleIcon size={14} />}
            active
          />
        </Breadcrumbs>
        <h1>Updates</h1>
        <Paragraph className={"block"}>
          Update the server, cache client updates and control client rollout.{" "}
          <InlineLink href={"/docs/guides/updating"}>Read the guide</InlineLink>
        </Paragraph>

        <section className={"mt-8"} aria-labelledby={"updates-server"}>
          <h2 id={"updates-server"} className={"text-lg"}>
            Server
          </h2>
          <div className={"mt-3 flex flex-wrap items-center gap-x-8 gap-y-3"}>
            <div>
              <div className={"text-xs text-nb-gray-400"}>Current</div>
              <div className={"font-mono"}>{current || "Unknown"}</div>
            </div>
            <div>
              <div className={"text-xs text-nb-gray-400"}>Latest cached</div>
              <div className={"font-mono"}>{latest || "Unknown"}</div>
            </div>
            {updateAvailable && data?.latest_release_url && (
              <InlineLink href={data.latest_release_url} target={"_blank"}>
                Release notes
              </InlineLink>
            )}
          </div>
          {data && !updateAvailable && latest && (
            <Callout
              className={"mt-4"}
              variant={"success"}
              icon={<CheckCircle2Icon size={14} className={"shrink-0 mt-0.5"} />}
            >
              The server is up to date.
            </Callout>
          )}
          {!updaterReady && (
            <Callout className={"mt-4"}>
              No updater is configured, so the server cannot update itself from
              this page. Enable the updater profile or upgrade manually, see the{" "}
              <InlineLink href={"/docs/guides/updating"}>updating guide</InlineLink>.
            </Callout>
          )}
          <div className={"mt-4 flex items-center gap-3"}>
            <Button
              variant={"primary"}
              size={"sm"}
              disabled={!updaterReady || !updateAvailable || busy}
              onClick={updateServer}
            >
              <ArrowUpCircleIcon size={14} />
              Update server
            </Button>
            <span aria-live={"polite"} className={"text-sm text-nb-gray-300"}>
              {busy ? progress ?? "Starting update" : ""}
            </span>
          </div>
          {result && (
            <Callout className={"mt-4"} variant={result.ok ? "success" : "error"}>
              {result.message}
            </Callout>
          )}
        </section>

        <section className={"mt-10"} aria-labelledby={"updates-mirror"}>
          <h2 id={"updates-mirror"} className={"text-lg"}>
            Client update mirror
          </h2>
          {data && !data.mirror.enabled ? (
            <Callout className={"mt-3"}>
              The mirror is off. Clients download updates from GitHub. Enable
              update_mirror in the server config to cache signed installers on
              this server.
            </Callout>
          ) : (
            <>
              <div className={"mt-3 grid grid-cols-2 gap-4 sm:grid-cols-4"}>
                <Stat label={"Versions"} value={data?.mirror.versions.join(", ") || "None"} />
                <Stat label={"Files"} value={String(data?.mirror.files ?? 0)} />
                <Stat label={"Last check"} value={when(data?.mirror.last_poll)} />
                <Stat label={"Next check"} value={when(data?.mirror.next_poll)} />
              </div>
              <div className={"mt-4"}>
                <div className={"flex justify-between text-xs text-nb-gray-400"}>
                  <span>Cache used</span>
                  <span>
                    {formatBytes(data?.mirror.bytes ?? 0)}
                    {data && data.mirror.max_bytes > 0
                      ? ` of ${formatBytes(data.mirror.max_bytes)}`
                      : ""}
                  </span>
                </div>
                <div
                  className={"mt-1 h-1.5 rounded bg-nb-gray-900 overflow-hidden"}
                  role={"progressbar"}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={meterPercent(
                    data?.mirror.bytes ?? 0,
                    data?.mirror.max_bytes ?? 0,
                  )}
                  aria-label={"Cache used"}
                >
                  <div
                    className={"h-full bg-netbirdy"}
                    style={{
                      width: `${meterPercent(
                        data?.mirror.bytes ?? 0,
                        data?.mirror.max_bytes ?? 0,
                      )}%`,
                    }}
                  />
                </div>
              </div>
              {data?.mirror.last_error && (
                <Callout className={"mt-4"} variant={"error"}>
                  {data.mirror.last_error}
                </Callout>
              )}
              <Button
                className={"mt-4"}
                variant={"secondary"}
                size={"sm"}
                onClick={checkNow}
              >
                <RefreshCwIcon size={14} />
                Check now
              </Button>
            </>
          )}
        </section>

        <section className={"mt-10"} aria-labelledby={"updates-clients"}>
          <h2 id={"updates-clients"} className={"text-lg"}>
            Client rollout
          </h2>
          <Paragraph className={"block mt-2"}>
            Rollout is set to{" "}
            <span className={"font-mono"}>
              {data?.client_rollout.auto_update_version ?? "disabled"}
            </span>
            {data?.client_rollout.auto_update_always
              ? " and installs automatically in the background."
              : "."}{" "}
            Change it under{" "}
            <InlineLink href={"/settings?tab=clients"}>Clients</InlineLink>. On a
            machine, run <span className={"font-mono"}>netbirdy update --check</span>{" "}
            or <span className={"font-mono"}>netbirdy update --install</span> to
            update by hand. Linux clients update through their package manager.
          </Paragraph>
        </section>
      </div>
    </Tabs.Content>
  );
}

const Stat = ({ label, value }: { label: string; value: string }) => (
  <div>
    <div className={"text-xs text-nb-gray-400"}>{label}</div>
    <div className={"text-sm"}>{value}</div>
  </div>
);
