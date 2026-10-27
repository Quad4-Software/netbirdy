// Types for the management security engine endpoints:
//   GET /security/status -> SecurityStatus
//   GET /security/events?limit=N -> SecurityEventList
//   PUT /security/mode   <- { mode }, -> SecurityModeResponse
// Field names match the Security* schemas generated into
// shared/management/http/api/types.gen.go (SecurityStatus, SecurityEvent,
// SecurityEventList, SecurityModeRequest/Response). A few alternate names are
// still accepted so the dashboard keeps working against a server that
// serializes the engine's own event type instead (see the normalizers in
// modules/settings/security.ts).

export const SecurityModes = {
  OFF: "off",
  OBSERVE: "observe",
  PREVENT: "prevent",
} as const;

export type SecurityMode = (typeof SecurityModes)[keyof typeof SecurityModes];

export interface SecurityRule {
  name: string;
  enabled: boolean;
  action: string;
}

export interface SecurityStatus {
  mode: SecurityMode;
  // "168h0m0s"-style Go duration in the generated schema; a nanosecond number
  // is tolerated for servers that serialize time.Duration directly.
  learning_period?: number | string;
  rules?: SecurityRule[];
  findings_total?: number;
  findings_by_rule?: Record<string, number>;
}

export interface SecurityModeResponse {
  mode: SecurityMode;
  // False means the change was persisted and takes effect after restart.
  applied?: boolean;
  note?: string;
}

// SecurityEvent is one anomaly-detection finding. The generated schema uses
// time, type, rule, action and src_ip; the engine's own event type uses ts,
// kind and action_taken, and a subject or ip summary may appear on older
// handlers. Read through the normalizers in modules/settings/security.ts
// rather than accessing fields directly.
export interface SecurityEvent {
  time?: string;
  ts?: string;
  type?: string;
  kind?: string;
  rule?: string;
  action?: string;
  action_taken?: string;
  severity?: string;
  account_id?: string;
  subject?: string;
  peer_id?: string;
  peer_name?: string;
  user_id?: string;
  src_ip?: string;
  ip?: string;
  reason?: string;
  would_prevent?: boolean;
  score?: number;
}

export interface SecurityEventList {
  events: SecurityEvent[];
}
