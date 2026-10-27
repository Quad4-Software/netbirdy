import type { BadgeVariants } from "@components/Badge";
import { isValidCIDR } from "@utils/ip";
import type { Permission } from "@/interfaces/Permission";
import type {
  AICrawlerPolicy,
  ProxyShieldConfig,
  ShieldMode,
} from "@/interfaces/ProxyShield";
import { AICrawlerPolicies, ShieldModes } from "@/interfaces/ProxyShield";

// Editable mirror of ProxyShieldConfig: allow_cidrs is one CIDR per line.
export type ShieldFormState = {
  mode: ShieldMode;
  allow_cidrs: string;
  ai_crawlers: AICrawlerPolicy;
  allow_search_engines: boolean;
  allow_local: boolean;
};

// allow_local defaults to true on the proxy when unset (loopback and private
// ranges bypass the shield), so a missing value maps to on.
export const shieldConfigToForm = (
  config?: ProxyShieldConfig | null,
): ShieldFormState => ({
  mode: config?.mode ?? ShieldModes.OBSERVE,
  allow_cidrs: (config?.allow_cidrs ?? []).join("\n"),
  ai_crawlers: config?.ai_crawlers ?? AICrawlerPolicies.DENY,
  allow_search_engines: config?.allow_search_engines ?? false,
  allow_local: config?.allow_local ?? true,
});

// Splits the textarea into trimmed non-empty lines.
export const shieldCidrLines = (text: string): string[] =>
  text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "");

// Non-empty lines that are not valid CIDRs; shown as a field error.
export const invalidShieldCidrs = (text: string): string[] =>
  shieldCidrLines(text).filter((line) => !isValidCIDR(line));

// The PUT body is the bare ProxyShieldConfig object; blank CIDR lines are
// dropped.
export const buildShieldPayload = (
  form: ShieldFormState,
): ProxyShieldConfig => ({
  mode: form.mode,
  allow_cidrs: shieldCidrLines(form.allow_cidrs),
  ai_crawlers: form.ai_crawlers,
  allow_search_engines: form.allow_search_engines,
  allow_local: form.allow_local,
});

// Stable value list for change tracking, ordered the same every call.
export const shieldFormValues = (
  form: ShieldFormState,
): (string | boolean)[] => [
  form.mode,
  form.allow_cidrs,
  form.ai_crawlers,
  form.allow_search_engines,
  form.allow_local,
];

export const isValidShieldMode = (v?: string): v is ShieldMode =>
  v === ShieldModes.OFF ||
  v === ShieldModes.OBSERVE ||
  v === ShieldModes.ENFORCE;

export const isValidAICrawlerPolicy = (v?: string): v is AICrawlerPolicy =>
  v === AICrawlerPolicies.DENY ||
  v === AICrawlerPolicies.ALLOW ||
  v === AICrawlerPolicies.CHALLENGE;

export const shieldModeBadgeVariant = (
  mode?: string,
): BadgeVariants["variant"] => {
  switch (mode) {
    case ShieldModes.ENFORCE:
      return "green";
    case ShieldModes.OBSERVE:
      return "yellow";
    default:
      return "gray";
  }
};

// The shield endpoints are part of the services surface: every connected
// proxy is managed through the reverse-proxy section, which the services
// permission gates.
export const canUpdateShield = (permission?: Permission): boolean =>
  !!permission?.update;
