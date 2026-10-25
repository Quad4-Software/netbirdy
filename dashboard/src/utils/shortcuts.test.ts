import { describe, expect, it } from "vitest";
import { matchGoTo } from "./shortcuts";

const seq = { last: "g", at: 1000 };
const key = (k: string) => ({ key: k, ctrlKey: false, metaKey: false });
const inp = document.createElement("input");

describe("matchGoTo", () => {
  it("g then p matches peers", () => {
    expect(matchGoTo(seq, key("p"), null, 1500)).toEqual({ consume: "match", path: "/peers" });
  });
  it("g then g matches groups", () => {
    expect(matchGoTo(seq, key("g"), null, 1500)).toEqual({ consume: "match", path: "/groups" });
  });
  it("times out after 1.2s", () => {
    expect(matchGoTo(seq, key("p"), null, 3000).consume).toBe("reset");
  });
  it("ignores editable targets", () => {
    expect(matchGoTo(seq, key("p"), inp, 1500).consume).toBe("reset");
  });
  it("ignores modifiers", () => {
    expect(matchGoTo(seq, { ...key("p"), ctrlKey: true }, null, 1500).consume).toBe("reset");
  });
  it("bare g is prefix", () => {
    expect(matchGoTo({ last: "", at: 0 }, key("g"), null, 1500).consume).toBe("prefix");
  });
});

describe("repeated g", () => {
  it("g then g resolves Groups", () => {
    expect(matchGoTo({ last: "g", at: 1000 }, key("g"), null, 1100)).toEqual({ consume: "match", path: "/groups" });
  });
  it("g g p does not navigate twice", () => {
    const r1 = matchGoTo({ last: "", at: 0 }, key("g"), null, 100);
    expect(r1.consume).toBe("prefix");
    const r2 = matchGoTo({ last: "g", at: 100 }, key("g"), null, 200);
    expect(r2.consume).toBe("match");
    const r3 = matchGoTo({ last: "", at: 0 }, key("p"), null, 300);
    expect(r3.consume).toBe("reset");
  });
});
