export interface ShortcutDef {
  keys: string;
  label: string;
  path: string;
  requires?: string; // permission key like "peers.read"
}

export const GO_TO_SEQUENCES: ShortcutDef[] = [
  { keys: "g p", label: "Peers", path: "/peers", requires: "peers.read" },
  { keys: "g g", label: "Groups", path: "/groups", requires: "groups.read" },
  { keys: "g a", label: "Access control policies", path: "/access-control", requires: "policies.read" },
  { keys: "g s", label: "Setup keys", path: "/setup-keys", requires: "setupKeys.read" },
  { keys: "g d", label: "DNS nameservers", path: "/dns/nameservers", requires: "dns.read" },
  { keys: "g r", label: "Network routes", path: "/network-routes", requires: "routes.read" },
  { keys: "g n", label: "Networks", path: "/networks", requires: "networks.read" },
  { keys: "g u", label: "Users", path: "/team/users", requires: "users.read" },
  { keys: "g e", label: "Audit events", path: "/events/audit", requires: "events.read" },
  { keys: "g c", label: "Control center", path: "/control-center" },
  { keys: "g t", label: "Settings", path: "/settings", requires: "settings.read" },
  { keys: "g o", label: "Documentation", path: "/docs" },
];

const GO_TIMEOUT_MS = 1200;

function isEditable(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false;
  return el.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(el.tagName);
}

// matchGoTo returns the target path when a `g` keypress sequence resolves.
// State is kept by the caller via a plain object.
export function matchGoTo(
  seq: { last: string; at: number },
  e: { key: string; ctrlKey: boolean; metaKey: boolean; shiftKey?: boolean; altKey?: boolean },
  target: EventTarget | null,
  now: number,
): { consume: "prefix" | "match" | "reset"; path?: string } {
  if (e.ctrlKey || e.metaKey || e.altKey) return { consume: "reset" };
  if (isEditable(target)) return { consume: "reset" };
  const k = e.key.toLowerCase();
  if (!/^[a-z]$/.test(k)) return { consume: "reset" };
  // A pending `g` resolves against "g <key>" first, so g g -> Groups
  // instead of restarting the sequence.
  if (seq.last === "g" && now - seq.at <= GO_TIMEOUT_MS) {
    const hit = GO_TO_SEQUENCES.find((s) => s.keys === `g ${k}`);
    if (hit) return { consume: "match", path: hit.path };
  }
  if (k === "g" && !e.shiftKey) return { consume: "prefix" };
  return { consume: "reset" };
}

export const SHEET_SHORTCUTS: { keys: string; label: string }[] = [
  { keys: "Ctrl+Shift+K", label: "Open command palette" },
  { keys: "/", label: "Open command palette" },
  { keys: "?", label: "Show this sheet" },
  { keys: "Esc", label: "Close dialog" },
  ...GO_TO_SEQUENCES.map((s) => ({ keys: s.keys, label: `Go to ${s.label}` })),
];
