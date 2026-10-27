import { describe, expect, it } from "vitest";
import {
  backupConfigToForm,
  backupFormValues,
  buildBackupPayload,
  describeSchedule,
  formatBackupTime,
  isMaskedSecret,
  isValidBackupInterval,
  MASKED_SECRET,
  secretForSubmit,
} from "./backups";

describe("isMaskedSecret", () => {
  it("matches the API mask, with or without padding", () => {
    expect(isMaskedSecret(MASKED_SECRET)).toBe(true);
    expect(isMaskedSecret("  ********  ")).toBe(true);
  });
  it("rejects real values and empty input", () => {
    expect(isMaskedSecret("hunter2")).toBe(false);
    expect(isMaskedSecret("")).toBe(false);
    expect(isMaskedSecret(undefined)).toBe(false);
  });
});

describe("secretForSubmit", () => {
  it("sends empty when the input holds the mask", () => {
    expect(secretForSubmit(MASKED_SECRET)).toBe("");
  });
  it("trims and passes through a typed value", () => {
    expect(secretForSubmit("  new-secret ")).toBe("new-secret");
  });
  it("keeps blank as blank so the stored value stays", () => {
    expect(secretForSubmit("")).toBe("");
    expect(secretForSubmit("   ")).toBe("");
  });
});

describe("isValidBackupInterval", () => {
  it("accepts empty and zero for manual-only backups", () => {
    expect(isValidBackupInterval("")).toBe(true);
    expect(isValidBackupInterval("   ")).toBe(true);
    expect(isValidBackupInterval("0")).toBe(true);
  });
  it("accepts Go-style durations", () => {
    for (const v of ["6h", "24h", "30m", "1h30m", "1.5h", "90s"]) {
      expect(isValidBackupInterval(v)).toBe(true);
    }
  });
  it("rejects malformed intervals", () => {
    for (const v of ["daily", "weekly", "6", "h", "6hh", "-6h", "6d", "6 h"]) {
      expect(isValidBackupInterval(v)).toBe(false);
    }
  });
});

describe("backupConfigToForm", () => {
  it("fills non-secret fields and blanks secrets", () => {
    const form = backupConfigToForm({
      s3_endpoint: "https://s3.example.com",
      s3_region: "eu-central-1",
      s3_bucket: "backups",
      s3_access_key: MASKED_SECRET,
      s3_secret_key: MASKED_SECRET,
      s3_prefix: "mgmt/",
      interval: "6h",
      encryption_key: MASKED_SECRET,
    });
    expect(form.s3_endpoint).toBe("https://s3.example.com");
    expect(form.s3_bucket).toBe("backups");
    expect(form.s3_access_key).toBe("");
    expect(form.s3_secret_key).toBe("");
    expect(form.encryption_key).toBe("");
  });
  it("returns empty strings when no config is stored", () => {
    const form = backupConfigToForm(null);
    expect(backupFormValues(form).every((v) => v === "")).toBe(true);
  });
});

describe("buildBackupPayload", () => {
  it("trims fields and converts masked secrets to empty", () => {
    const payload = buildBackupPayload({
      s3_endpoint: " https://s3.example.com ",
      s3_region: "eu-central-1",
      s3_bucket: "backups",
      s3_access_key: MASKED_SECRET,
      s3_secret_key: " typed-secret ",
      s3_prefix: "mgmt/",
      interval: "6h",
      encryption_key: "",
    });
    expect(payload.s3_endpoint).toBe("https://s3.example.com");
    expect(payload.s3_access_key).toBe("");
    expect(payload.s3_secret_key).toBe("typed-secret");
    expect(payload.encryption_key).toBe("");
  });
});

describe("describeSchedule", () => {
  it("describes configured intervals", () => {
    expect(describeSchedule("6h")).toBe("Every 6h");
  });
  it("treats empty, zero and invalid intervals as manual only", () => {
    for (const v of [undefined, "", "0", "bogus"]) {
      expect(describeSchedule(v)).toBe("Manual only");
    }
  });
});

describe("formatBackupTime", () => {
  it("renders missing and zero times as Never", () => {
    expect(formatBackupTime(undefined)).toBe("Never");
    expect(formatBackupTime("")).toBe("Never");
    expect(formatBackupTime("0001-01-01T00:00:00Z")).toBe("Never");
  });
  it("renders a relative time for real timestamps", () => {
    expect(formatBackupTime("2020-01-01T00:00:00Z")).toContain("ago");
  });
});
