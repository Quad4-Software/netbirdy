import Breadcrumbs from "@components/Breadcrumbs";
import Button from "@components/Button";
import FancyToggleSwitch from "@components/FancyToggleSwitch";
import HelpText from "@components/HelpText";
import InlineLink from "@components/InlineLink";
import { Input } from "@components/Input";
import { Label } from "@components/Label";
import { notify } from "@components/Notification";
import Paragraph from "@components/Paragraph";
import { useHasChanges } from "@hooks/useHasChanges";
import * as Tabs from "@radix-ui/react-tabs";
import { useApiCall } from "@utils/api";
import { BugIcon, ChartNoAxesCombined, ExternalLinkIcon } from "lucide-react";
import React, { useMemo, useState } from "react";
import { useSWRConfig } from "swr";
import SettingsIcon from "@/assets/icons/SettingsIcon";
import { usePermissions } from "@/contexts/PermissionsProvider";
import { Account } from "@/interfaces/Account";
import { docsLink } from "@/modules/docs/links";
import {
  isValidErrorReportingDsn,
  resolveErrorReportingConfig,
  syncErrorReporting,
  writeErrorReportingConfig,
} from "@/modules/error-reporting/errorReporting";

type Props = {
  account: Account;
};

export default function MetricsTab({ account }: Readonly<Props>) {
  const { permission } = usePermissions();
  const { mutate } = useSWRConfig();
  const saveRequest = useApiCall<Account>("/accounts/" + account.id, true);

  const [metricsPushEnabled, setMetricsPushEnabled] = useState(
    account.settings?.metrics_push_enabled ?? false,
  );

  const initialErrorReporting = useMemo(
    () => resolveErrorReportingConfig(account),
    [account],
  );
  const [errorReportingEnabled, setErrorReportingEnabled] = useState(
    initialErrorReporting.enabled,
  );
  const [errorReportingDsn, setErrorReportingDsn] = useState(
    initialErrorReporting.dsn,
  );

  const { hasChanges, updateRef } = useHasChanges([
    errorReportingEnabled,
    errorReportingDsn,
  ]);

  const dsnError = useMemo(() => {
    if (errorReportingDsn === "") {
      return errorReportingEnabled
        ? "A DSN is required when error reporting is enabled"
        : "";
    }
    if (!isValidErrorReportingDsn(errorReportingDsn)) {
      return "Please enter a valid DSN, e.g. https://<key>@o0.ingest.sentry.io/0";
    }
    return "";
  }, [errorReportingDsn, errorReportingEnabled]);

  const saveErrorReporting = async () => {
    const config = {
      enabled: errorReportingEnabled,
      dsn: errorReportingDsn.trim(),
    };

    notify({
      title: "Error Reporting",
      description: `Error reporting settings successfully updated.`,
      promise: saveRequest
        .put({
          id: account.id,
          settings: {
            ...account.settings,
            error_reporting_enabled: config.enabled,
            error_reporting_dsn: config.dsn,
          },
        })
        .then(() => {
          writeErrorReportingConfig(account.id, config);
          void syncErrorReporting(config);
          mutate("/accounts");
          updateRef([errorReportingEnabled, errorReportingDsn]);
        }),
      loadingMessage: "Updating error reporting settings...",
    });
  };

  const toggleMetricsPush = async (toggle: boolean) => {
    notify({
      title: "Metrics",
      description: `Metrics push successfully ${
        toggle ? "enabled" : "disabled"
      }.`,
      promise: saveRequest
        .put({
          id: account.id,
          settings: {
            ...account.settings,
            metrics_push_enabled: toggle,
          },
        })
        .then(() => {
          setMetricsPushEnabled(toggle);
          mutate("/accounts");
        }),
      loadingMessage: "Updating metrics setting...",
    });
  };

  return (
    <Tabs.Content value={"metrics"}>
      <div className={"p-default py-6 max-w-2xl"}>
        <Breadcrumbs>
          <Breadcrumbs.Item
            href={"/settings"}
            label={"Settings"}
            icon={<SettingsIcon size={13} />}
          />
          <Breadcrumbs.Item
            href={"/settings?tab=metrics"}
            label={"Metrics"}
            icon={<ChartNoAxesCombined size={14} />}
            active
          />
        </Breadcrumbs>
        <div className={"flex items-start justify-between"}>
          <div>
            <h1>Metrics</h1>
            <Paragraph className={"block"}>
              Share client performance metrics to help us improve NetBirdy.{" "}
              <InlineLink
                href={docsLink("https://docs.netbird.io/manage/client-metrics")}
                target={"_blank"}
              >
                Learn more
                <ExternalLinkIcon size={12} />
              </InlineLink>
            </Paragraph>
          </div>
          <Button
            variant={"primary"}
            disabled={
              !hasChanges || !permission.settings.update || dsnError !== ""
            }
            onClick={saveErrorReporting}
            data-testid={"save-metrics-settings"}
          >
            Save Changes
          </Button>
        </div>

        <FancyToggleSwitch
          className={"mt-6"}
          value={metricsPushEnabled}
          onChange={toggleMetricsPush}
          label={
            <>
              <ChartNoAxesCombined size={15} />
              Share performance metrics
            </>
          }
          helpText={
            "When enabled, clients will periodically send performance data to help us identify and fix issues."
          }
          disabled={!permission.settings.update}
        />

        <div className={"mt-10"}>
          <Label>
            <BugIcon size={15} />
            Error Reporting
          </Label>
          <HelpText>
            Send uncaught dashboard errors to your own Sentry-compatible
            endpoint. Works with Sentry, GlitchTip and Bugsink. Paste the
            project DSN below and save. Reporting stays off until you enable it
            with a valid DSN.
          </HelpText>
          <FancyToggleSwitch
            className={"mt-2"}
            value={errorReportingEnabled}
            onChange={setErrorReportingEnabled}
            data-testid={"error-reporting-enabled"}
            label={"Enable error reporting"}
            helpText={
              "When enabled, uncaught errors in this dashboard are sent to the configured Sentry endpoint. No data is sent while disabled."
            }
            disabled={!permission.settings.update}
          />
          <div className={"mt-3"}>
            <Input
              placeholder={"https://<key>@o0.ingest.sentry.io/0"}
              errorTooltip={true}
              errorTooltipPosition={"top"}
              error={dsnError}
              value={errorReportingDsn}
              disabled={!permission.settings.update}
              onChange={(e) => setErrorReportingDsn(e.target.value)}
              data-testid={"error-reporting-dsn-input"}
            />
          </div>
        </div>
      </div>
    </Tabs.Content>
  );
}
