import type { Account } from "@/interfaces/Account";

export interface ErrorReportingConfig {
  enabled: boolean;
  dsn: string;
}

// The management API does not persist the error_reporting_* account settings
// yet, so the dashboard mirrors them per account in localStorage. When a
// backend learns the fields, the server value wins and the mirror becomes a
// fallback for deployments running an older management server.
const storageKey = (accountId: string) =>
  `netbirdy-error-reporting:${accountId}`;

// isValidErrorReportingDsn accepts only http(s) URLs. A Sentry DSN carries
// the credentials in the userinfo part, so anything else is a typo.
export const isValidErrorReportingDsn = (dsn: string): boolean => {
  try {
    const url = new URL(dsn);
    return url.protocol === "https:" || url.protocol === "http:";
  } catch {
    return false;
  }
};

export const readErrorReportingConfig = (
  accountId: string,
): ErrorReportingConfig | undefined => {
  if (typeof window === "undefined") return undefined;
  try {
    const raw = window.localStorage.getItem(storageKey(accountId));
    if (!raw) return undefined;
    const parsed = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null) return undefined;
    return {
      enabled: parsed.enabled === true,
      dsn: typeof parsed.dsn === "string" ? parsed.dsn : "",
    };
  } catch {
    return undefined;
  }
};

export const writeErrorReportingConfig = (
  accountId: string,
  config: ErrorReportingConfig,
): void => {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(storageKey(accountId), JSON.stringify(config));
  } catch {
    // Storage can be unavailable (private mode, quota). The setting then
    // only lives for the current session.
  }
};

// resolveErrorReportingConfig merges the account settings with the local
// mirror. Server values take precedence once the backend persists them.
export const resolveErrorReportingConfig = (
  account?: Account,
): ErrorReportingConfig => {
  const stored = account ? readErrorReportingConfig(account.id) : undefined;
  return {
    enabled:
      account?.settings?.error_reporting_enabled ?? stored?.enabled ?? false,
    dsn: account?.settings?.error_reporting_dsn ?? stored?.dsn ?? "",
  };
};

let activeDsn: string | undefined;

// syncErrorReporting brings the Sentry client in line with the desired
// config: init when enabled with a valid DSN, close when disabled or the DSN
// changed. It is idempotent and swallows every failure; error reporting must
// never take the dashboard down with it.
export const syncErrorReporting = async (
  config: ErrorReportingConfig,
): Promise<void> => {
  const shouldBeActive = config.enabled && isValidErrorReportingDsn(config.dsn);
  if (shouldBeActive && activeDsn === config.dsn) return;
  if (!shouldBeActive && activeDsn === undefined) return;

  try {
    const Sentry = await import("@sentry/browser");
    if (activeDsn !== undefined) {
      await Sentry.close();
      activeDsn = undefined;
    }
    if (!shouldBeActive) return;
    Sentry.init({
      dsn: config.dsn,
      release: process.env.NEXT_PUBLIC_DASHBOARD_VERSION,
      environment: process.env.APP_ENV || process.env.NODE_ENV,
      beforeSend(event) {
        event.tags = { ...event.tags, service: "dashboard" };
        return event;
      },
    });
    activeDsn = config.dsn;
  } catch (e) {
    console.warn("Error reporting setup failed", e);
  }
};
