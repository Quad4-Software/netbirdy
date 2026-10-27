"use client";

import Badge from "@components/Badge";
import Button from "@components/Button";
import { Callout } from "@components/Callout";
import FancyToggleSwitch from "@components/FancyToggleSwitch";
import HelpText from "@components/HelpText";
import { Label } from "@components/Label";
import { notify } from "@components/Notification";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@components/Select";
import SkeletonTable from "@components/skeletons/SkeletonTable";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@components/table/Table";
import { Textarea } from "@components/Textarea";
import { useHasChanges } from "@hooks/useHasChanges";
import useFetchApi, { useApiCall } from "@utils/api";
import { RefreshCwIcon, ShieldIcon, ShieldOffIcon } from "lucide-react";
import React, { useMemo, useState } from "react";
import { usePermissions } from "@/contexts/PermissionsProvider";
import type {
  AICrawlerPolicy,
  ProxyShieldProxy,
  ProxyShieldState,
  ProxyShieldUnbanResult,
  ProxyShieldUpdateResult,
  ShieldMode,
} from "@/interfaces/ProxyShield";
import { AICrawlerPolicies, ShieldModes } from "@/interfaces/ProxyShield";
import type { ShieldFormState } from "@/modules/reverse-proxy/shield/shield";
import {
  buildShieldPayload,
  canUpdateShield,
  invalidShieldCidrs,
  shieldConfigToForm,
  shieldFormValues,
  shieldModeBadgeVariant,
} from "@/modules/reverse-proxy/shield/shield";

const SHIELD_MODE_OPTIONS: { value: ShieldMode; label: string }[] = [
  { value: ShieldModes.OFF, label: "Off" },
  { value: ShieldModes.OBSERVE, label: "Observe" },
  { value: ShieldModes.ENFORCE, label: "Enforce" },
];

const AI_CRAWLER_OPTIONS: { value: AICrawlerPolicy; label: string }[] = [
  { value: AICrawlerPolicies.DENY, label: "Deny" },
  { value: AICrawlerPolicies.ALLOW, label: "Allow" },
  { value: AICrawlerPolicies.CHALLENGE, label: "Challenge" },
];

export default function ShieldSettings() {
  const {
    data: shield,
    isLoading,
    error,
    mutate: refresh,
  } = useFetchApi<ProxyShieldState>("/proxy/shield", true, true, true, {
    shouldRetryOnError: false,
  });
  const { permission } = usePermissions();
  const canUpdate = canUpdateShield(permission?.services);

  if (isLoading && !shield) {
    return (
      <div className={"p-default pt-0 max-w-3xl"}>
        <SkeletonTable />
      </div>
    );
  }

  if (!shield) {
    return (
      <div className={"p-default pt-0 max-w-3xl"}>
        <Callout variant={"error"}>
          {error
            ? "The proxy shield status could not be loaded from this server. The shield endpoints require an owner or admin role and may need a newer management version."
            : "The proxy shield status could not be loaded."}
        </Callout>
      </div>
    );
  }

  return (
    <ShieldForm
      key={JSON.stringify(shield.config)}
      shield={shield}
      canUpdate={canUpdate}
      refresh={() => void refresh()}
    />
  );
}

type FormProps = {
  shield: ProxyShieldState;
  canUpdate: boolean;
  refresh: () => void;
};

function ShieldForm({ shield, canUpdate, refresh }: Readonly<FormProps>) {
  const saveRequest = useApiCall<ProxyShieldUpdateResult>(
    "/proxy/shield",
    true,
  );
  const [form, setForm] = useState<ShieldFormState>(() =>
    shieldConfigToForm(shield.config),
  );

  const { hasChanges, updateRef } = useHasChanges(shieldFormValues(form));

  const cidrErrors = useMemo(
    () => invalidShieldCidrs(form.allow_cidrs),
    [form.allow_cidrs],
  );

  const save = () => {
    notify({
      title: "Shield",
      description: "Shield settings were applied to every connected proxy.",
      loadingMessage: "Saving shield settings...",
      promise: saveRequest.put(buildShieldPayload(form)).then(() => {
        updateRef(shieldFormValues(form));
        refresh();
      }),
    });
  };

  return (
    <div className={"p-default pt-0 max-w-3xl"}>
      <section aria-labelledby={"shield-config"}>
        <div className={"flex items-start justify-between"}>
          <div>
            <h2 id={"shield-config"} className={"text-lg"}>
              Configuration
            </h2>
            <HelpText>
              Applied live to every connected proxy, no restart needed.
            </HelpText>
          </div>
          {canUpdate && (
            <Button
              variant={"primary"}
              disabled={!hasChanges || cidrErrors.length > 0}
              onClick={save}
              data-testid={"save-shield-settings"}
            >
              Save Changes
            </Button>
          )}
        </div>

        <div className={"mt-4 flex flex-col gap-6"}>
          <div>
            <Label>Mode</Label>
            <HelpText className={"mt-1"}>
              <b className={"text-white"}>Observe</b> scores sources and logs
              what it would block. <b className={"text-white"}>Enforce</b>{" "}
              applies challenges, tarpits and bans.
            </HelpText>
            <div className={"mt-2"}>
              <Select
                value={form.mode}
                onValueChange={(v) =>
                  setForm((f) => ({ ...f, mode: v as ShieldMode }))
                }
                disabled={!canUpdate}
              >
                <SelectTrigger
                  className={"w-[220px]"}
                  data-testid={"shield-mode-trigger"}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SHIELD_MODE_OPTIONS.map((option) => (
                    <SelectItem
                      key={option.value}
                      value={option.value}
                      data-testid={`shield-mode-${option.value}`}
                    >
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {form.mode === ShieldModes.ENFORCE && (
              <Callout className={"mt-4"} variant={"warning"}>
                Enforce mode challenges and bans sources on every connected
                proxy. Run observe first and check the status below before
                enabling it.
              </Callout>
            )}
          </div>

          <FancyToggleSwitch
            value={form.allow_local}
            onChange={(v) => setForm((f) => ({ ...f, allow_local: v }))}
            disabled={!canUpdate}
            data-testid={"shield-allow-local"}
            label={
              <>
                <ShieldIcon size={15} />
                Allow local sources
              </>
            }
            helpText={
              <>
                Loopback and private-range sources (LAN, link-local) bypass the
                shield entirely.
              </>
            }
          />

          <FancyToggleSwitch
            value={form.allow_search_engines}
            onChange={(v) =>
              setForm((f) => ({ ...f, allow_search_engines: v }))
            }
            disabled={!canUpdate}
            data-testid={"shield-allow-search-engines"}
            label={
              <>
                <ShieldIcon size={15} />
                Allow search engines
              </>
            }
            helpText={
              <>
                Verified search-engine crawlers (Google, Bing and similar)
                bypass the shield. Claims that fail verification count as a
                strong negative signal.
              </>
            }
          />

          <div>
            <Label>AI crawlers</Label>
            <HelpText className={"mt-1"}>
              What to do with known AI training crawlers.
            </HelpText>
            <div className={"mt-2"}>
              <Select
                value={form.ai_crawlers}
                onValueChange={(v) =>
                  setForm((f) => ({ ...f, ai_crawlers: v as AICrawlerPolicy }))
                }
                disabled={!canUpdate}
              >
                <SelectTrigger
                  className={"w-[220px]"}
                  data-testid={"shield-ai-crawlers"}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {AI_CRAWLER_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div>
            <Label>Allowed CIDRs</Label>
            <HelpText className={"mt-1"}>
              Sources in these ranges bypass the shield. One CIDR per line.
            </HelpText>
            <Textarea
              className={"mt-2 font-mono text-xs"}
              rows={4}
              resize
              placeholder={"100.64.0.0/10"}
              value={form.allow_cidrs}
              onChange={(e) =>
                setForm((f) => ({ ...f, allow_cidrs: e.target.value }))
              }
              disabled={!canUpdate}
              error={
                cidrErrors.length > 0
                  ? `Invalid CIDR: ${cidrErrors.join(", ")}`
                  : undefined
              }
              data-testid={"shield-allow-cidrs"}
            />
          </div>
        </div>
      </section>

      <section className={"mt-10"} aria-labelledby={"shield-proxies"}>
        <div className={"flex items-center justify-between"}>
          <h2 id={"shield-proxies"} className={"text-lg"}>
            Connected proxies
          </h2>
          <Button
            variant={"secondary"}
            size={"sm"}
            onClick={refresh}
            data-testid={"shield-proxies-refresh"}
          >
            <RefreshCwIcon size={14} />
            Refresh
          </Button>
        </div>
        <ProxyShieldTable
          proxies={shield.proxies ?? []}
          canUpdate={canUpdate}
          refresh={refresh}
        />
      </section>
    </div>
  );
}

type TableProps = {
  proxies: ProxyShieldProxy[];
  canUpdate: boolean;
  refresh: () => void;
};

function ProxyShieldTable({ proxies, canUpdate, refresh }: TableProps) {
  const unbanRequest = useApiCall<ProxyShieldUnbanResult>(
    "/proxy/shield/unban",
    true,
  );
  const [unbanning, setUnbanning] = useState<string>("");

  const unban = (ip: string) => {
    setUnbanning(ip);
    notify({
      title: "Shield",
      description: `${ip} was unbanned.`,
      loadingMessage: `Unbanning ${ip}...`,
      promise: unbanRequest
        .post({}, `?ip=${encodeURIComponent(ip)}`)
        .then(() => refresh())
        .finally(() => setUnbanning("")),
    });
  };

  if (proxies.length === 0) {
    return (
      <div className={"mt-4 text-sm text-nb-gray-400"}>
        No proxies are connected, so nothing reports shield status yet.
      </div>
    );
  }

  return (
    <div className={"mt-4"}>
      <Table minimal>
        <TableHeader minimal>
          <TableRow minimal>
            <TableHead minimal>Proxy</TableHead>
            <TableHead minimal>Mode</TableHead>
            <TableHead minimal>Banned</TableHead>
            <TableHead minimal>Tarpits</TableHead>
            <TableHead minimal>Tracked</TableHead>
            <TableHead minimal>Banned IPs</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {proxies.map((proxy) => {
            const status = proxy.status;
            const bannedIps = status?.banned_ips ?? [];
            return (
              <TableRow minimal key={proxy.proxy_id}>
                <TableCell minimal className={"font-mono text-xs"}>
                  {proxy.proxy_id}
                </TableCell>
                <TableCell minimal>
                  {status?.mode ? (
                    <Badge
                      variant={shieldModeBadgeVariant(status.mode)}
                      size={"xs"}
                    >
                      {status.mode}
                    </Badge>
                  ) : (
                    "-"
                  )}
                </TableCell>
                <TableCell minimal>{status?.banned ?? 0}</TableCell>
                <TableCell minimal>{status?.active_tarpits ?? 0}</TableCell>
                <TableCell minimal>{status?.tracked_sources ?? 0}</TableCell>
                <TableCell minimal>
                  {bannedIps.length === 0 ? (
                    "-"
                  ) : (
                    <div className={"flex flex-col gap-2"}>
                      {bannedIps.map((ip) => (
                        <div
                          key={ip}
                          className={
                            "flex items-center gap-3 font-mono text-xs"
                          }
                        >
                          <ShieldOffIcon
                            size={12}
                            className={"text-red-400 shrink-0"}
                          />
                          <span>{ip}</span>
                          {canUpdate && (
                            <Button
                              variant={"secondary"}
                              size={"sm"}
                              disabled={unbanning === ip}
                              onClick={() => unban(ip)}
                              data-testid={`shield-unban-${ip}`}
                            >
                              {unbanning === ip ? "Unbanning..." : "Unban"}
                            </Button>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}
