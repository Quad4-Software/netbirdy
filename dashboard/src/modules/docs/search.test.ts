import { describe, expect, it } from "vitest";
import { searchDocs } from "./search";

const pages = [
  { slug: "offline-mode", section: "guides", title: "Offline Mode", description: "keep peers up", headings: [{ id: "x", text: "Snapshot", level: 2 }], text: "the network map snapshot keeps peers connected" },
  { slug: "quickstart", section: "getting-started", title: "Quickstart", description: "podman", headings: [], text: "run podman compose up" },
];

describe("searchDocs", () => {
  it("ranks title hits above body", () => {
    const r = searchDocs(pages, "offline");
    expect(r[0].title).toBe("Offline Mode");
  });
  it("finds body terms", () => {
    const r = searchDocs(pages, "snapshot");
    expect(r.length).toBe(1);
  });
  it("empty query returns nothing", () => {
    expect(searchDocs(pages, "  ")).toEqual([]);
  });
});
