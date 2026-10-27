import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Account } from "@/interfaces/Account";
import {
  isValidErrorReportingDsn,
  readErrorReportingConfig,
  resolveErrorReportingConfig,
  syncErrorReporting,
  writeErrorReportingConfig,
} from "./errorReporting";

const init = vi.fn();
const close = vi.fn(() => Promise.resolve(true));

vi.mock("@sentry/browser", () => ({
  init: (...args: unknown[]) => init(...args),
  close: () => close(),
}));

const account = (settings?: Partial<Account["settings"]>): Account =>
  ({
    id: "acc-1",
    settings: { ...settings },
  }) as Account;

const validDsn = "https://abc123@o0.ingest.sentry.io/0";

beforeEach(async () => {
  window.localStorage.clear();
  init.mockClear();
  close.mockClear();
  // Bring the module back to the inactive state no matter what the
  // previous test left behind.
  await syncErrorReporting({ enabled: false, dsn: "" });
  close.mockClear();
});

describe("isValidErrorReportingDsn", () => {
  it("accepts http and https URLs", () => {
    expect(isValidErrorReportingDsn(validDsn)).toBe(true);
    expect(isValidErrorReportingDsn("http://glitchtip.internal/1")).toBe(true);
    expect(isValidErrorReportingDsn("http://localhost:8000/2")).toBe(true);
  });
  it("rejects non-URLs, other schemes and empty input", () => {
    for (const dsn of [
      "",
      "not a dsn",
      "javascript:alert(1)",
      "ftp://example.com/1",
      "o0.ingest.sentry.io/0",
    ]) {
      expect(isValidErrorReportingDsn(dsn)).toBe(false);
    }
  });
});

describe("localStorage mirror", () => {
  it("round-trips a config", () => {
    writeErrorReportingConfig("acc-1", { enabled: true, dsn: validDsn });
    expect(readErrorReportingConfig("acc-1")).toEqual({
      enabled: true,
      dsn: validDsn,
    });
  });
  it("returns undefined for unknown accounts and broken values", () => {
    expect(readErrorReportingConfig("nope")).toBeUndefined();
    window.localStorage.setItem("netbirdy-error-reporting:acc-1", "not json");
    expect(readErrorReportingConfig("acc-1")).toBeUndefined();
  });
});

describe("resolveErrorReportingConfig", () => {
  it("defaults to disabled with no account", () => {
    expect(resolveErrorReportingConfig()).toEqual({ enabled: false, dsn: "" });
  });
  it("falls back to the stored config", () => {
    writeErrorReportingConfig("acc-1", { enabled: true, dsn: validDsn });
    expect(resolveErrorReportingConfig(account())).toEqual({
      enabled: true,
      dsn: validDsn,
    });
  });
  it("prefers account settings over the stored config", () => {
    writeErrorReportingConfig("acc-1", { enabled: false, dsn: "https://x/1" });
    const resolved = resolveErrorReportingConfig(
      account({
        error_reporting_enabled: true,
        error_reporting_dsn: validDsn,
      }),
    );
    expect(resolved).toEqual({ enabled: true, dsn: validDsn });
  });
});

describe("syncErrorReporting", () => {
  it("does nothing when disabled", async () => {
    await syncErrorReporting({ enabled: false, dsn: validDsn });
    expect(init).not.toHaveBeenCalled();
  });
  it("does nothing when the DSN is invalid", async () => {
    await syncErrorReporting({ enabled: true, dsn: "not a dsn" });
    expect(init).not.toHaveBeenCalled();
  });
  it("initializes once for an enabled valid config", async () => {
    const config = { enabled: true, dsn: validDsn };
    await syncErrorReporting(config);
    await syncErrorReporting(config);
    expect(init).toHaveBeenCalledTimes(1);
    expect(init.mock.calls[0][0]).toMatchObject({ dsn: validDsn });
  });
  it("tags events with service=dashboard in beforeSend", async () => {
    await syncErrorReporting({ enabled: true, dsn: validDsn });
    const { beforeSend } = init.mock.calls[0][0];
    const event = beforeSend({ tags: { a: "1" } });
    expect(event.tags).toEqual({ a: "1", service: "dashboard" });
  });
  it("closes the client when disabled after being active", async () => {
    await syncErrorReporting({ enabled: true, dsn: validDsn });
    await syncErrorReporting({ enabled: false, dsn: validDsn });
    expect(close).toHaveBeenCalled();
    await syncErrorReporting({ enabled: true, dsn: validDsn });
    expect(init).toHaveBeenCalledTimes(2);
  });
});
