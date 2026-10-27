import Badge from "@components/Badge";
import Breadcrumbs from "@components/Breadcrumbs";
import Button from "@components/Button";
import { Callout } from "@components/Callout";
import HelpText from "@components/HelpText";
import { notify } from "@components/Notification";
import Paragraph from "@components/Paragraph";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@components/Select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@components/table/Table";
import * as Tabs from "@radix-ui/react-tabs";
import useFetchApi, { useApiCall } from "@utils/api";
import dayjs from "dayjs";
import { RefreshCwIcon, ShieldIcon } from "lucide-react";
import React, { useMemo, useState } from "react";
import Skeleton from "react-loading-skeleton";
import SettingsIcon from "@/assets/icons/SettingsIcon";
import type {
  SecurityEventList,
  SecurityMode,
  SecurityModeResponse,
  SecurityStatus,
} from "@/interfaces/Security";
import { SecurityModes } from "@/interfaces/Security";
import {
  findingsForRule,
  formatLearningPeriod,
  labelize,
  securityEventAction,
  securityEventKind,
  securityEventSubject,
  securityEventTime,
  securityModeBadgeVariant,
  severityBadgeVariant,
  sortedSecurityRules,
} from "@/modules/settings/security";

const EVENT_LIMITS = ["50", "100", "200", "500"];

const SECURITY_MODE_OPTIONS: { value: SecurityMode; label: string }[] = [
  { value: SecurityModes.OFF, label: "Off" },
  { value: SecurityModes.OBSERVE, label: "Observe" },
  { value: SecurityModes.PREVENT, label: "Prevent" },
];

export default function SecurityTab() {
  const {
    data: status,
    isLoading,
    error: statusError,
    mutate: refreshStatus,
  } = useFetchApi<SecurityStatus>("/security/status", true, true, true, {
    shouldRetryOnError: false,
  });
  const [limit, setLimit] = useState("200");
  const {
    data: eventsData,
    isLoading: eventsLoading,
    error: eventsError,
    mutate: refreshEvents,
  } = useFetchApi<SecurityEventList>(
    `/security/events?limit=${limit}`,
    true,
    true,
    true,
    {
      shouldRetryOnError: false,
    },
  );

  const events = eventsData?.events ?? [];
  const unavailable = statusError !== undefined && status === undefined;

  return (
    <Tabs.Content value={"security"}>
      <div className={"p-default py-6 max-w-3xl"}>
        <Breadcrumbs>
          <Breadcrumbs.Item
            href={"/settings"}
            label={"Settings"}
            icon={<SettingsIcon size={13} />}
          />
          <Breadcrumbs.Item
            href={"/settings?tab=security"}
            label={"Security"}
            icon={<ShieldIcon size={14} />}
            active
          />
        </Breadcrumbs>
        <h1>Security</h1>
        <Paragraph className={"block"}>
          The security engine watches management events for anomalies such as
          new peer IPs, impossible travel and authentication bursts. Findings
          are always logged; prevention is optional.
        </Paragraph>
        {isLoading && !status ? (
          <div className={"mt-6"}>
            <Skeleton height={42} count={3} className={"mb-3"} />
          </div>
        ) : unavailable ? (
          <Callout className={"mt-6"} variant={"error"}>
            The security status could not be loaded from this server. The
            security endpoints require an owner or admin role and may need a
            newer management version.
          </Callout>
        ) : status ? (
          <SecurityContent
            status={status}
            events={events}
            eventsLoading={eventsLoading}
            eventsUnavailable={eventsError !== undefined}
            limit={limit}
            onLimitChange={setLimit}
            refresh={() => {
              void refreshStatus();
              void refreshEvents();
            }}
            refreshStatus={() => void refreshStatus()}
          />
        ) : null}
      </div>
    </Tabs.Content>
  );
}

type ContentProps = {
  status: SecurityStatus;
  events: SecurityEventList["events"];
  eventsLoading: boolean;
  eventsUnavailable: boolean;
  limit: string;
  onLimitChange: (limit: string) => void;
  refresh: () => void;
  refreshStatus: () => void;
};

function SecurityContent({
  status,
  events,
  eventsLoading,
  eventsUnavailable,
  limit,
  onLimitChange,
  refresh,
  refreshStatus,
}: Readonly<ContentProps>) {
  const modeRequest = useApiCall<SecurityModeResponse>("/security/mode", true);
  const [modeNote, setModeNote] = useState<string>("");
  const rules = useMemo(
    () => sortedSecurityRules(status.rules),
    [status.rules],
  );

  const changeMode = (mode: string) => {
    if (mode === status.mode) return;
    setModeNote("");
    notify({
      title: "Security",
      description: `Security mode set to ${mode}.`,
      loadingMessage: "Updating security mode...",
      promise: modeRequest.put({ mode }).then((res) => {
        if (res?.note) setModeNote(res.note);
        refreshStatus();
      }),
    });
  };

  return (
    <>
      <section className={"mt-8"} aria-labelledby={"security-status"}>
        <h2 id={"security-status"} className={"text-lg"}>
          Status
        </h2>
        <div className={"mt-4 flex flex-wrap items-center gap-x-8 gap-y-3"}>
          <div>
            <div className={"text-xs text-nb-gray-400"}>Mode</div>
            <Badge
              variant={securityModeBadgeVariant(status.mode)}
              className={"mt-1 w-fit"}
            >
              {labelize(status.mode)}
            </Badge>
          </div>
          <Stat label={"Findings"} value={String(status.findings_total ?? 0)} />
          <Stat
            label={"Learning period"}
            value={formatLearningPeriod(status.learning_period)}
          />
        </div>
        {rules.length > 0 && (
          <div className={"mt-4"}>
            <Table minimal>
              <TableHeader minimal>
                <TableRow minimal>
                  <TableHead minimal>Rule</TableHead>
                  <TableHead minimal>Enabled</TableHead>
                  <TableHead minimal>Action</TableHead>
                  <TableHead minimal>Findings</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rules.map((rule) => (
                  <TableRow minimal key={rule.name}>
                    <TableCell minimal className={"font-mono text-xs"}>
                      {rule.name}
                    </TableCell>
                    <TableCell minimal>{rule.enabled ? "Yes" : "No"}</TableCell>
                    <TableCell minimal>{labelize(rule.action)}</TableCell>
                    <TableCell minimal>
                      {findingsForRule(status.findings_by_rule, rule.name)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </section>

      <section className={"mt-10"} aria-labelledby={"security-mode"}>
        <h2 id={"security-mode"} className={"text-lg"}>
          Mode
        </h2>
        <HelpText className={"mt-1"}>
          <b className={"text-white"}>Observe</b> collects findings without
          acting on them. <b className={"text-white"}>Prevent</b> applies the
          action configured on each rule.
        </HelpText>
        <div className={"mt-3 flex items-center gap-4"}>
          <Select value={status.mode} onValueChange={changeMode}>
            <SelectTrigger
              className={"w-[220px]"}
              data-testid={"security-mode-trigger"}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {SECURITY_MODE_OPTIONS.map((option) => (
                <SelectItem
                  key={option.value}
                  value={option.value}
                  data-testid={`security-mode-${option.value}`}
                >
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Callout className={"mt-4"} variant={"warning"}>
          Prevent mode can block peers and expire logins when a rule fires. Run
          observe first and review the findings below before enabling it.
        </Callout>
        {modeNote !== "" && (
          <Callout
            className={"mt-4"}
            variant={"info"}
            data-testid={"security-mode-note"}
          >
            {modeNote}
          </Callout>
        )}
      </section>

      <section className={"mt-10"} aria-labelledby={"security-events"}>
        <div className={"flex items-center justify-between"}>
          <h2 id={"security-events"} className={"text-lg"}>
            Findings
          </h2>
          <div className={"flex items-center gap-3"}>
            <Select value={limit} onValueChange={onLimitChange}>
              <SelectTrigger
                className={"w-[110px]"}
                data-testid={"security-events-limit"}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {EVENT_LIMITS.map((l) => (
                  <SelectItem key={l} value={l}>
                    {l}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              variant={"secondary"}
              size={"sm"}
              onClick={refresh}
              disabled={eventsLoading}
              data-testid={"security-events-refresh"}
            >
              <RefreshCwIcon size={14} />
              Refresh
            </Button>
          </div>
        </div>
        {status.mode === SecurityModes.OFF ? (
          <Callout className={"mt-4"} variant={"info"}>
            The security engine is off. Switch to observe to start collecting
            findings without blocking anything.
          </Callout>
        ) : eventsUnavailable ? (
          <Callout className={"mt-4"} variant={"error"}>
            The security events could not be loaded from this server.
          </Callout>
        ) : events.length === 0 ? (
          <div className={"mt-4 text-sm text-nb-gray-400"}>
            No findings yet.
          </div>
        ) : (
          <div className={"mt-4"}>
            <Table minimal>
              <TableHeader minimal>
                <TableRow minimal>
                  <TableHead minimal>Time</TableHead>
                  <TableHead minimal>Severity</TableHead>
                  <TableHead minimal>Finding</TableHead>
                  <TableHead minimal>Action</TableHead>
                  <TableHead minimal>Subject</TableHead>
                  <TableHead minimal>Reason</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {events.map((event, index) => (
                  <TableRow minimal key={index}>
                    <TableCell minimal className={"whitespace-nowrap"}>
                      {securityEventTime(event)
                        ? dayjs(securityEventTime(event)).format(
                            "MMM D, YYYY [at] h:mm A",
                          )
                        : "-"}
                    </TableCell>
                    <TableCell minimal>
                      {event.severity ? (
                        <Badge
                          variant={severityBadgeVariant(event.severity)}
                          size={"xs"}
                        >
                          {labelize(event.severity)}
                        </Badge>
                      ) : (
                        "-"
                      )}
                    </TableCell>
                    <TableCell minimal className={"font-mono text-xs"}>
                      {securityEventKind(event) || "-"}
                    </TableCell>
                    <TableCell minimal>
                      {event.would_prevent ? (
                        <Badge variant={"yellow"} size={"xs"}>
                          Would prevent
                        </Badge>
                      ) : securityEventAction(event) ? (
                        <Badge variant={"gray"} size={"xs"}>
                          {labelize(securityEventAction(event))}
                        </Badge>
                      ) : (
                        "Logged"
                      )}
                    </TableCell>
                    <TableCell minimal className={"font-mono text-xs"}>
                      {securityEventSubject(event) || "-"}
                    </TableCell>
                    <TableCell minimal>{event.reason || "-"}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
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
