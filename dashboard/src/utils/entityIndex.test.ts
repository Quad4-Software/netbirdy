import { describe, expect, it } from "vitest";
import { Group } from "@/interfaces/Group";
import { User } from "@/interfaces/User";
import {
  indexGroupsById,
  indexPeersToGroups,
  indexUsersById,
} from "./entityIndex";

const groups: Group[] = [
  {
    id: "g1",
    name: "one",
    peers: [
      { id: "p1", name: "p1" },
      { id: "p2", name: "p2" },
    ],
  },
  { id: "g2", name: "two", peers: [{ id: "p2", name: "p2" }] },
  { id: "g3", name: "three" },
];

const users: User[] = [
  { id: "u1", name: "a" } as User,
  { id: "u2", name: "b" } as User,
];

describe("entityIndex", () => {
  it("indexes groups by id", () => {
    const idx = indexGroupsById(groups);
    expect(idx.get("g2")?.name).toBe("two");
    expect(idx.size).toBe(3);
  });

  it("indexes users by id", () => {
    const idx = indexUsersById(users);
    expect(idx.get("u1")?.name).toBe("a");
    expect(idx.get("missing")).toBeUndefined();
  });

  it("maps peers to all groups containing them", () => {
    const idx = indexPeersToGroups(groups);
    expect(idx.get("p1")?.map((g) => g.id)).toEqual(["g1"]);
    expect(idx.get("p2")?.map((g) => g.id)).toEqual(["g1", "g2"]);
    expect(idx.get("unknown")).toBeUndefined();
  });

  it("returns the same index instance for the same array", () => {
    expect(indexGroupsById(groups)).toBe(indexGroupsById(groups));
    expect(indexPeersToGroups(groups)).toBe(indexPeersToGroups(groups));
  });

  it("rebuilds for a new array identity", () => {
    const copy = groups.slice();
    const first = indexGroupsById(groups);
    const second = indexGroupsById(copy);
    expect(second).not.toBe(first);
    expect(second.get("g1")?.name).toBe("one");
  });

  it("handles undefined input", () => {
    expect(indexGroupsById(undefined).size).toBe(0);
    expect(indexPeersToGroups(undefined).size).toBe(0);
    expect(indexUsersById(undefined).size).toBe(0);
  });
});
