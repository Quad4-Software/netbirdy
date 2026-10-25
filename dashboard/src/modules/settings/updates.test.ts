import { describe, expect, it, vi } from "vitest";
import {
  compareVersions,
  isNewer,
  isZeroTime,
  meterPercent,
  normalizeVersion,
  UpdatesInfo,
  waitForServerVersion,
} from "@/modules/settings/updates";

const info = (over: Partial<UpdatesInfo> = {}): UpdatesInfo => ({
  server_version: "1.0.0",
  latest_version: "1.1.0",
  latest_release_url: "",
  mirror: {
    enabled: true,
    versions: [],
    files: 0,
    bytes: 0,
    max_bytes: 0,
    last_poll: "0001-01-01T00:00:00Z",
    next_poll: "0001-01-01T00:00:00Z",
  },
  updater: { available: true },
  client_rollout: { auto_update_version: "disabled", auto_update_always: false },
  ...over,
});

describe("version helpers", () => {
  it("normalizes a leading v and whitespace", () => {
    expect(normalizeVersion(" v1.2.3 ")).toBe("1.2.3");
    expect(normalizeVersion(undefined)).toBe("");
  });

  it("compares numerically, not lexically", () => {
    expect(compareVersions("1.10.0", "1.9.9")).toBe(1);
    expect(compareVersions("v1.2.3", "1.2.3")).toBe(0);
    expect(compareVersions("1.2.3", "1.2.4")).toBe(-1);
  });

  it("ranks a release above its prerelease", () => {
    expect(compareVersions("1.2.3", "1.2.3-rc.1")).toBe(1);
    expect(compareVersions("1.2.3-rc.1", "1.2.3")).toBe(-1);
    expect(compareVersions("1.2.3-rc.2", "1.2.3-rc.1")).toBe(1);
  });

  it("reports newer only when both versions are known", () => {
    expect(isNewer("1.1.0", "1.0.0")).toBe(true);
    expect(isNewer("1.0.0", "1.0.0")).toBe(false);
    expect(isNewer("", "1.0.0")).toBe(false);
    expect(isNewer("1.1.0", "")).toBe(false);
  });

  it("detects the zero time", () => {
    expect(isZeroTime("0001-01-01T00:00:00Z")).toBe(true);
    expect(isZeroTime(undefined)).toBe(true);
    expect(isZeroTime("2026-10-25T12:00:00Z")).toBe(false);
  });

  it("caps the cache meter", () => {
    expect(meterPercent(50, 100)).toBe(50);
    expect(meterPercent(500, 100)).toBe(100);
    expect(meterPercent(5, 0)).toBe(0);
  });
});

describe("waitForServerVersion", () => {
  const harness = () => {
    let t = 0;
    return {
      now: () => t,
      sleep: vi.fn(async (ms: number) => {
        t += ms;
      }),
    };
  };

  it("succeeds when the server reports the target", async () => {
    const h = harness();
    const fetchInfo = vi
      .fn()
      .mockResolvedValueOnce(info())
      .mockResolvedValueOnce(info({ server_version: "v1.1.0" }));
    const res = await waitForServerVersion({ ...h, fetchInfo, target: "1.1.0" });
    expect(res.phase).toBe("success");
    expect(fetchInfo).toHaveBeenCalledTimes(2);
  });

  it("tolerates errors while the server restarts", async () => {
    const h = harness();
    const progress: string[] = [];
    const fetchInfo = vi
      .fn()
      .mockRejectedValueOnce(new Error("502"))
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce(info({ server_version: "1.1.0" }));
    const res = await waitForServerVersion({
      ...h,
      fetchInfo,
      target: "v1.1.0",
      onProgress: (s) => progress.push(s),
    });
    expect(res.phase).toBe("success");
    expect(progress).toEqual([
      "Waiting for the server to restart",
      "Waiting for the server to restart",
    ]);
  });

  it("fails when the updater reports an error", async () => {
    const h = harness();
    const fetchInfo = vi.fn().mockResolvedValue(
      info({ updater: { available: true, status: { state: "failed", error: "rollback done" } } }),
    );
    const res = await waitForServerVersion({ ...h, fetchInfo, target: "1.1.0" });
    expect(res).toEqual({ phase: "failed", message: "rollback done" });
  });

  it("times out and backs off between polls", async () => {
    const h = harness();
    const fetchInfo = vi.fn().mockResolvedValue(info());
    const res = await waitForServerVersion({
      ...h,
      fetchInfo,
      target: "1.1.0",
      timeoutMs: 60000,
    });
    expect(res.phase).toBe("timeout");
    const delays = h.sleep.mock.calls.map((c) => c[0]);
    expect(delays[0]).toBe(2000);
    expect(delays[1]).toBe(3000);
    expect(Math.max(...delays)).toBeLessThanOrEqual(10000);
  });

  it("shows updater progress text between polls", async () => {
    const h = harness();
    const progress: string[] = [];
    const fetchInfo = vi
      .fn()
      .mockResolvedValueOnce(info({ updater: { available: true, status: { state: "running", step: "pulling images" } } }))
      .mockResolvedValueOnce(info({ server_version: "1.1.0" }));
    await waitForServerVersion({ ...h, fetchInfo, target: "1.1.0", onProgress: (s) => progress.push(s) });
    expect(progress).toEqual(["pulling images"]);
  });
});
