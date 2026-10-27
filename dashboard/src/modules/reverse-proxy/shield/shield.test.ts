import { describe, expect, it } from "vitest";
import type { Permission } from "@/interfaces/Permission";
import {
  buildShieldPayload,
  canUpdateShield,
  invalidShieldCidrs,
  isValidAICrawlerPolicy,
  isValidShieldMode,
  shieldCidrLines,
  shieldConfigToForm,
  shieldModeBadgeVariant,
} from "@/modules/reverse-proxy/shield/shield";

describe("shieldConfigToForm", () => {
  it("maps a stored config into form state", () => {
    const form = shieldConfigToForm({
      mode: "enforce",
      allow_cidrs: ["10.0.0.0/8", "192.168.0.0/16"],
      ai_crawlers: "challenge",
      allow_search_engines: true,
      allow_local: false,
    });
    expect(form).toEqual({
      mode: "enforce",
      allow_cidrs: "10.0.0.0/8\n192.168.0.0/16",
      ai_crawlers: "challenge",
      allow_search_engines: true,
      allow_local: false,
    });
  });

  it("defaults to observe mode, deny crawlers and allow_local", () => {
    expect(shieldConfigToForm(null)).toEqual({
      mode: "observe",
      allow_cidrs: "",
      ai_crawlers: "deny",
      allow_search_engines: false,
      allow_local: true,
    });
    expect(shieldConfigToForm({ mode: "off" }).allow_local).toBe(true);
  });
});

describe("shieldCidrLines", () => {
  it("trims lines and drops empties", () => {
    expect(shieldCidrLines(" 10.0.0.0/8 \n\n\n192.168.0.0/16\n")).toEqual([
      "10.0.0.0/8",
      "192.168.0.0/16",
    ]);
    expect(shieldCidrLines("")).toEqual([]);
  });
});

describe("invalidShieldCidrs", () => {
  it("returns only the invalid lines", () => {
    expect(
      invalidShieldCidrs("10.0.0.0/8\nnot-a-cidr\n::1/128\n1.2.3.4"),
    ).toEqual(["not-a-cidr", "1.2.3.4"]);
    expect(invalidShieldCidrs("")).toEqual([]);
    expect(invalidShieldCidrs("10.0.0.0/8")).toEqual([]);
  });
});

describe("buildShieldPayload", () => {
  it("returns the bare config and drops blank CIDR lines", () => {
    const payload = buildShieldPayload({
      mode: "enforce",
      allow_cidrs: "10.0.0.0/8\n\n",
      ai_crawlers: "allow",
      allow_search_engines: false,
      allow_local: true,
    });
    expect(payload).toEqual({
      mode: "enforce",
      allow_cidrs: ["10.0.0.0/8"],
      ai_crawlers: "allow",
      allow_search_engines: false,
      allow_local: true,
    });
  });
});

describe("isValidShieldMode", () => {
  it("accepts off, observe and enforce", () => {
    expect(isValidShieldMode("off")).toBe(true);
    expect(isValidShieldMode("observe")).toBe(true);
    expect(isValidShieldMode("enforce")).toBe(true);
    expect(isValidShieldMode("prevent")).toBe(false);
    expect(isValidShieldMode(undefined)).toBe(false);
  });
});

describe("isValidAICrawlerPolicy", () => {
  it("accepts deny, allow and challenge", () => {
    expect(isValidAICrawlerPolicy("deny")).toBe(true);
    expect(isValidAICrawlerPolicy("allow")).toBe(true);
    expect(isValidAICrawlerPolicy("challenge")).toBe(true);
    expect(isValidAICrawlerPolicy("block")).toBe(false);
    expect(isValidAICrawlerPolicy(undefined)).toBe(false);
  });
});

describe("shieldModeBadgeVariant", () => {
  it("maps modes to badge variants", () => {
    expect(shieldModeBadgeVariant("enforce")).toBe("green");
    expect(shieldModeBadgeVariant("observe")).toBe("yellow");
    expect(shieldModeBadgeVariant("off")).toBe("gray");
    expect(shieldModeBadgeVariant("bogus")).toBe("gray");
  });
});

describe("canUpdateShield", () => {
  const permission = (update: boolean): Permission => ({
    create: update,
    read: true,
    update,
    delete: update,
  });

  it("follows the services update grant", () => {
    expect(canUpdateShield(permission(true))).toBe(true);
    expect(canUpdateShield(permission(false))).toBe(false);
    expect(canUpdateShield(undefined)).toBe(false);
  });
});
