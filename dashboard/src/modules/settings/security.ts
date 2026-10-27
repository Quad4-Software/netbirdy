import type { BadgeVariants } from "@components/Badge";
import type {
  SecurityEvent,
  SecurityMode,
  SecurityRule,
} from "@/interfaces/Security";
import { SecurityModes } from "@/interfaces/Security";

// The event payload tolerates two naming schemes: the security package's own
// JSON tags (ts, type, rule, action_taken, peer_name, src_ip) and friendlier
// REST names (time, kind, action, subject, ip) the handler may use instead.

export const securityEventTime = (e: SecurityEvent): string =>
  e.time ?? e.ts ?? "";

export const securityEventKind = (e: SecurityEvent): string =>
  e.kind ?? e.rule ?? e.type ?? "";

export const securityEventAction = (e: SecurityEvent): string =>
  e.action ?? e.action_taken ?? "";

export const securityEventSubject = (e: SecurityEvent): string =>
  e.subject ?? e.peer_name ?? e.user_id ?? e.src_ip ?? e.ip ?? "";

export const isValidSecurityMode = (v?: string): v is SecurityMode =>
  v === SecurityModes.OFF ||
  v === SecurityModes.OBSERVE ||
  v === SecurityModes.PREVENT;

export const securityModeBadgeVariant = (
  mode?: string,
): BadgeVariants["variant"] => {
  switch (mode) {
    case SecurityModes.PREVENT:
      return "green";
    case SecurityModes.OBSERVE:
      return "yellow";
    default:
      return "gray";
  }
};

export const severityBadgeVariant = (
  severity?: string,
): BadgeVariants["variant"] => {
  switch (severity) {
    case "high":
      return "red";
    case "medium":
      return "yellow";
    case "low":
      return "blue";
    default:
      return "gray";
  }
};

// Turns snake_case identifiers from the engine (peer_new_ip,
// require_approval) into labels for display.
export const labelize = (value?: string): string => {
  if (!value) return "";
  const words = value.replace(/[_-]+/g, " ").trim();
  return words.charAt(0).toUpperCase() + words.slice(1);
};

// learning_period is a Go time.Duration. Serialized as nanoseconds it arrives
// as a number; some encoders emit a duration string like "168h0m0s" instead.
export const formatLearningPeriod = (value?: number | string): string => {
  if (value === undefined || value === null || value === "") return "-";
  if (typeof value === "string") return value;
  if (!Number.isFinite(value) || value <= 0) return "-";
  const minutes = Math.round(value / 60e9);
  if (minutes < 60) return `${minutes} min`;
  const hours = Math.round(minutes / 60);
  if (hours < 48) return `${hours} h`;
  const days = Math.round(hours / 24);
  return days === 1 ? "1 day" : `${days} days`;
};

// Stable ordering for the rules table: enabled rules first, then by name.
export const sortedSecurityRules = (rules?: SecurityRule[]): SecurityRule[] =>
  [...(rules ?? [])].sort((a, b) => {
    if (a.enabled !== b.enabled) return a.enabled ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

// Findings for one rule from the findings_by_rule map; zero when absent.
export const findingsForRule = (
  findings: Record<string, number> | undefined,
  rule: string,
): number => findings?.[rule] ?? 0;
