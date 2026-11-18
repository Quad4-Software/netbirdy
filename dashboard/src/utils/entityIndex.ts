import { Group, GroupPeer } from "@/interfaces/Group";
import { User } from "@/interfaces/User";

/**
 * Shared entity indexes keyed by array identity.
 *
 * The groups and users arrays come from SWR and keep a stable reference
 * between fetches, so a WeakMap lets every consumer share one index build
 * instead of each hook re-running O(list × members) scans per row.
 */

const groupByIdCache = new WeakMap<readonly Group[], Map<string, Group>>();
const peersToGroupsCache = new WeakMap<
  readonly Group[],
  Map<string, Group[]>
>();
const userByIdCache = new WeakMap<readonly User[], Map<string, User>>();

/** Index groups by id. */
export function indexGroupsById(
  groups: readonly Group[] | undefined,
): Map<string, Group> {
  if (!groups) return new Map();
  const cached = groupByIdCache.get(groups);
  if (cached) return cached;
  const map = new Map<string, Group>();
  for (const g of groups) {
    if (g.id != null) map.set(g.id, g);
  }
  groupByIdCache.set(groups, map);
  return map;
}

/** Index group membership: peer id -> groups containing that peer. */
export function indexPeersToGroups(
  groups: readonly Group[] | undefined,
): Map<string, Group[]> {
  if (!groups) return new Map();
  const cached = peersToGroupsCache.get(groups);
  if (cached) return cached;
  const map = new Map<string, Group[]>();
  for (const g of groups) {
    for (const p of g.peers ?? []) {
      const id = (p as GroupPeer).id;
      if (id == null) continue;
      const list = map.get(id);
      if (list) {
        list.push(g);
      } else {
        map.set(id, [g]);
      }
    }
  }
  peersToGroupsCache.set(groups, map);
  return map;
}

/** Index users by id. */
export function indexUsersById(
  users: readonly User[] | undefined,
): Map<string, User> {
  if (!users) return new Map();
  const cached = userByIdCache.get(users);
  if (cached) return cached;
  const map = new Map<string, User>();
  for (const u of users) {
    if (u.id != null) map.set(u.id, u);
  }
  userByIdCache.set(users, map);
  return map;
}
