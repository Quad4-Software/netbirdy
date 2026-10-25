import { describe, expect, it } from "vitest";
import { buildPaletteItems, filterPaletteItems, isEditableTarget, matchPaletteShortcut } from "./palette";

describe("palette", () => {
  it("builds static actions plus nav items", () => {
    const items = buildPaletteItems();
    expect(items.some((i) => i.id === "theme-dark")).toBe(true);
    expect(items.some((i) => i.id === "sign-out")).toBe(true);
  });

  it("filters by query case-insensitively", () => {
    const items = buildPaletteItems();
    expect(filterPaletteItems(items, "THEME").length).toBe(3);
    expect(filterPaletteItems(items, "sign").length).toBe(1);
    expect(filterPaletteItems(items, "zzz").length).toBe(0);
  });

  it("matches Ctrl+Shift+K and Cmd+Shift+K", () => {
    const e = { key: "k", ctrlKey: true, metaKey: false, shiftKey: true };
    expect(matchPaletteShortcut(e, null)).toBe(true);
    expect(matchPaletteShortcut({ ...e, ctrlKey: false, metaKey: true }, null)).toBe(true);
    expect(matchPaletteShortcut({ ...e, shiftKey: false }, null)).toBe(false);
  });

  it("matches / only outside editable targets", () => {
    const e = { key: "/", ctrlKey: false, metaKey: false, shiftKey: false };
    expect(matchPaletteShortcut(e, null)).toBe(true);
    const input = document.createElement("input");
    expect(matchPaletteShortcut(e, input)).toBe(false);
  });

  it("detects editable targets", () => {
    expect(isEditableTarget(document.createElement("input"))).toBe(true);
    expect(isEditableTarget(document.createElement("div"))).toBe(false);
  });
});

describe("readNavItems", () => {
  it("returns leaf items only, no concatenated group labels", async () => {
    const { readNavItems, filterPaletteItems } = await import("./palette");
    document.body.innerHTML = `
      <ul>
        <li data-nav-item="/access-control"><div>Access Control
          <ul>
            <li data-nav-item="/access-control/policies">Policies</li>
            <li data-nav-item="/groups">Groups</li>
            <li data-nav-item="/posture-checks">Posture Checks</li>
          </ul></div></li>
        <li data-nav-item="/peers"><div>Peers</div></li>
      </ul>`;
    const items = readNavItems(document);
    const labels = items.map((i) => i.label);
    expect(items).toHaveLength(4);
    expect(labels).not.toContain("Access Control");
    expect(labels).toEqual(["Policies", "Groups", "Posture Checks", "Peers"]);
    const themeHits = filterPaletteItems(
      [...items, ...buildPaletteItems().filter((i) => i.kind === "action")],
      "theme",
    );
    expect(themeHits.map((i) => i.id)).toEqual(["theme-light", "theme-dark", "theme-system"]);
  });
});
