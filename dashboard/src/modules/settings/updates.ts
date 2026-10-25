export type UpdaterStatus = {
  state?: string;
  step?: string;
  message?: string;
  version?: string;
  error?: string;
};

export type UpdatesInfo = {
  server_version: string;
  latest_version: string;
  latest_release_url: string;
  mirror: {
    enabled: boolean;
    versions: string[];
    files: number;
    bytes: number;
    max_bytes: number;
    last_poll: string;
    next_poll: string;
    last_error?: string;
  };
  updater: {
    available: boolean;
    status?: UpdaterStatus;
    error?: string;
  };
  client_rollout: {
    auto_update_version: string;
    auto_update_always: boolean;
  };
};

export const normalizeVersion = (v?: string): string =>
  (v ?? "").trim().replace(/^v/i, "");

const parse = (v: string) => {
  const [core, pre = ""] = normalizeVersion(v).split(/-(.+)/);
  const nums = core.split(".").map((n) => parseInt(n, 10));
  return { nums: nums.map((n) => (Number.isNaN(n) ? 0 : n)), pre };
};

export function compareVersions(a: string, b: string): number {
  const pa = parse(a);
  const pb = parse(b);
  const len = Math.max(pa.nums.length, pb.nums.length);
  for (let i = 0; i < len; i++) {
    const d = (pa.nums[i] ?? 0) - (pb.nums[i] ?? 0);
    if (d !== 0) return d > 0 ? 1 : -1;
  }
  if (pa.pre === pb.pre) return 0;
  if (pa.pre === "") return 1;
  if (pb.pre === "") return -1;
  return pa.pre > pb.pre ? 1 : -1;
}

export const isNewer = (latest: string, current: string): boolean =>
  normalizeVersion(latest) !== "" &&
  normalizeVersion(current) !== "" &&
  compareVersions(latest, current) > 0;

export const isZeroTime = (iso?: string): boolean =>
  !iso || iso.startsWith("0001-");

export const meterPercent = (bytes: number, max: number): number =>
  max > 0 ? Math.min(100, Math.round((bytes / max) * 100)) : 0;

export type WaitPhase = "success" | "failed" | "timeout";

export type WaitResult = { phase: WaitPhase; message: string };

type WaitOptions = {
  fetchInfo: () => Promise<UpdatesInfo>;
  target: string;
  sleep: (ms: number) => Promise<void>;
  now: () => number;
  timeoutMs?: number;
  minDelayMs?: number;
  maxDelayMs?: number;
  onProgress?: (text: string) => void;
};

export async function waitForServerVersion({
  fetchInfo,
  target,
  sleep,
  now,
  timeoutMs = 5 * 60 * 1000,
  minDelayMs = 2000,
  maxDelayMs = 10000,
  onProgress,
}: WaitOptions): Promise<WaitResult> {
  const start = now();
  let delay = minDelayMs;
  const want = normalizeVersion(target);
  while (now() - start < timeoutMs) {
    await sleep(delay);
    delay = Math.min(maxDelayMs, Math.round(delay * 1.5));
    try {
      const info = await fetchInfo();
      if (normalizeVersion(info.server_version) === want) {
        return { phase: "success", message: `Server is running ${want}.` };
      }
      const st = info.updater.status;
      if (st?.state === "failed" || st?.error) {
        return {
          phase: "failed",
          message: st.error || st.message || "The update failed.",
        };
      }
      onProgress?.(st?.message || st?.step || st?.state || "Updating");
    } catch {
      onProgress?.("Waiting for the server to restart");
    }
  }
  return {
    phase: "timeout",
    message: "The server did not report the new version in time.",
  };
}
