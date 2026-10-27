import { describe, expect, it } from "vitest";
import type { SecurityEvent } from "@/interfaces/Security";
import {
  findingsForRule,
  formatLearningPeriod,
  isValidSecurityMode,
  labelize,
  securityEventAction,
  securityEventKind,
  securityEventSubject,
  securityEventTime,
  securityModeBadgeVariant,
  severityBadgeVariant,
  sortedSecurityRules,
} from "@/modules/settings/security";

describe("securityEventTime", () => {
  it("prefers time over ts", () => {
    const e: SecurityEvent = { time: "2026-01-01T00:00:00Z", ts: "x" };
    expect(securityEventTime(e)).toBe("2026-01-01T00:00:00Z");
  });
  it("falls back to ts", () => {
    expect(securityEventTime({ ts: "2026-01-01T00:00:00Z" })).toBe(
      "2026-01-01T00:00:00Z",
    );
  });
  it("returns empty string when absent", () => {
    expect(securityEventTime({})).toBe("");
  });
});

describe("securityEventKind", () => {
  it("prefers kind, then rule, then type", () => {
    expect(securityEventKind({ kind: "k", rule: "r", type: "t" })).toBe("k");
    expect(securityEventKind({ rule: "r", type: "t" })).toBe("r");
    expect(securityEventKind({ type: "t" })).toBe("t");
    expect(securityEventKind({})).toBe("");
  });
});

describe("securityEventAction", () => {
  it("prefers action over action_taken", () => {
    expect(securityEventAction({ action: "a", action_taken: "t" })).toBe("a");
    expect(securityEventAction({ action_taken: "t" })).toBe("t");
    expect(securityEventAction({})).toBe("");
  });
});

describe("securityEventSubject", () => {
  it("prefers subject, then peer_name, then user_id, then src_ip, then ip", () => {
    expect(securityEventSubject({ subject: "s", peer_name: "p" })).toBe("s");
    expect(securityEventSubject({ peer_name: "p", user_id: "u" })).toBe("p");
    expect(securityEventSubject({ user_id: "u", src_ip: "1.1.1.1" })).toBe("u");
    expect(securityEventSubject({ src_ip: "1.1.1.1", ip: "2.2.2.2" })).toBe(
      "1.1.1.1",
    );
    expect(securityEventSubject({ ip: "2.2.2.2" })).toBe("2.2.2.2");
    expect(securityEventSubject({})).toBe("");
  });
});

describe("isValidSecurityMode", () => {
  it("accepts the three modes", () => {
    expect(isValidSecurityMode("off")).toBe(true);
    expect(isValidSecurityMode("observe")).toBe(true);
    expect(isValidSecurityMode("prevent")).toBe(true);
  });
  it("rejects unknown and empty values", () => {
    expect(isValidSecurityMode("enforce")).toBe(false);
    expect(isValidSecurityMode("")).toBe(false);
    expect(isValidSecurityMode(undefined)).toBe(false);
  });
});

describe("securityModeBadgeVariant", () => {
  it("maps modes to badge variants", () => {
    expect(securityModeBadgeVariant("prevent")).toBe("green");
    expect(securityModeBadgeVariant("observe")).toBe("yellow");
    expect(securityModeBadgeVariant("off")).toBe("gray");
    expect(securityModeBadgeVariant("bogus")).toBe("gray");
    expect(securityModeBadgeVariant(undefined)).toBe("gray");
  });
});

describe("severityBadgeVariant", () => {
  it("maps severities to badge variants", () => {
    expect(severityBadgeVariant("high")).toBe("red");
    expect(severityBadgeVariant("medium")).toBe("yellow");
    expect(severityBadgeVariant("low")).toBe("blue");
    expect(severityBadgeVariant("info")).toBe("gray");
    expect(severityBadgeVariant(undefined)).toBe("gray");
  });
});

describe("labelize", () => {
  it("replaces underscores and dashes with spaces and capitalizes", () => {
    expect(labelize("peer_new_ip")).toBe("Peer new ip");
    expect(labelize("require_approval")).toBe("Require approval");
    expect(labelize("auth-failure-burst")).toBe("Auth failure burst");
  });
  it("returns empty string for empty input", () => {
    expect(labelize("")).toBe("");
    expect(labelize(undefined)).toBe("");
  });
});

describe("formatLearningPeriod", () => {
  it("renders nanosecond durations", () => {
    expect(formatLearningPeriod(168 * 3.6e12)).toBe("7 days"); // 168h in ns
    expect(formatLearningPeriod(36e11)).toBe("1 h"); // 1h in ns
    expect(formatLearningPeriod(30 * 6e10)).toBe("30 min");
    expect(formatLearningPeriod(2 * 24 * 3.6e12)).toBe("2 days");
  });
  it("passes duration strings through", () => {
    expect(formatLearningPeriod("168h")).toBe("168h");
  });
  it("handles missing and invalid values", () => {
    expect(formatLearningPeriod(undefined)).toBe("-");
    expect(formatLearningPeriod("")).toBe("-");
    expect(formatLearningPeriod(0)).toBe("-");
    expect(formatLearningPeriod(-5)).toBe("-");
  });
});

describe("sortedSecurityRules", () => {
  it("sorts enabled first, then alphabetically", () => {
    const rules = sortedSecurityRules([
      { name: "b_rule", enabled: false, action: "alert" },
      { name: "c_rule", enabled: true, action: "alert" },
      { name: "a_rule", enabled: true, action: "alert" },
      { name: "a_off", enabled: false, action: "alert" },
    ]);
    expect(rules.map((r) => r.name)).toEqual([
      "a_rule",
      "c_rule",
      "a_off",
      "b_rule",
    ]);
  });
  it("handles missing input", () => {
    expect(sortedSecurityRules(undefined)).toEqual([]);
  });
});

describe("findingsForRule", () => {
  it("returns the count for the rule", () => {
    const map = { peer_new_ip: 3 };
    expect(findingsForRule(map, "peer_new_ip")).toBe(3);
    expect(findingsForRule(map, "missing")).toBe(0);
    expect(findingsForRule(undefined, "peer_new_ip")).toBe(0);
  });
});
