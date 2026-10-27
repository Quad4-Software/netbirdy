// Types for the proxy shield endpoints:
//   GET  /proxy/shield        -> ProxyShieldState
//   PUT  /proxy/shield        <- { config }, -> ProxyShieldUpdateResult
//   POST /proxy/shield/unban?ip=x.x.x.x -> ProxyShieldUnbanResult
// Field names match the ProxyShield* schemas generated into
// shared/management/http/api/types.gen.go and the YAML keys of
// proxy/internal/shield.

export const ShieldModes = {
  OFF: "off",
  OBSERVE: "observe",
  ENFORCE: "enforce",
} as const;

export type ShieldMode = (typeof ShieldModes)[keyof typeof ShieldModes];

export const AICrawlerPolicies = {
  DENY: "deny",
  ALLOW: "allow",
  CHALLENGE: "challenge",
} as const;

export type AICrawlerPolicy =
  (typeof AICrawlerPolicies)[keyof typeof AICrawlerPolicies];

export interface ProxyShieldConfig {
  mode: ShieldMode;
  // Source ranges that always bypass the shield.
  allow_cidrs?: string[];
  ai_crawlers?: AICrawlerPolicy;
  allow_search_engines?: boolean;
  allow_local?: boolean;
}

// Shield status last reported by one proxy.
export interface ProxyShieldStatus {
  mode?: string;
  banned?: number;
  active_tarpits?: number;
  tracked_sources?: number;
  banned_ips?: string[];
}

// One connected proxy and its last reported shield status.
export interface ProxyShieldProxy {
  proxy_id: string;
  status?: ProxyShieldStatus;
}

// Shield configuration and per-proxy status.
export interface ProxyShieldState {
  config: ProxyShieldConfig;
  proxies?: ProxyShieldProxy[];
}

export interface ProxyShieldUpdateResult {
  config: ProxyShieldConfig;
  // Connected proxies the new config was pushed to immediately; the rest
  // pick it up on their next sync-stream message.
  pushed?: number;
}

export interface ProxyShieldUnbanResult {
  ip?: string;
  pushed?: number;
}
