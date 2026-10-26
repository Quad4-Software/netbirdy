import { describe, expect, it } from "vitest";
import { safeHttpsUrl } from "./UpdatesTab";

describe("safeHttpsUrl", () => {
  it("accepts https", () => {
    expect(safeHttpsUrl("https://x.test/r")).toBe(true);
  });
  it("rejects javascript, data, http, garbage, empty", () => {
    for (const u of ["javascript:alert(1)", "data:text/html,x", "http://x.test", "not a url", "", undefined]) {
      expect(safeHttpsUrl(u as string)).toBe(false);
    }
  });
});
