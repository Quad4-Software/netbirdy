import {
  auditEvents,
  cities,
  countries,
  groups,
  identityProviders,
  invites,
  jobs,
  peers,
  setupKeys,
  tokens,
  trafficEvents,
  users,
} from "./core-data";
import { PEER } from "./ids";
import type { Handler, HandlerContext } from "./index";

const byId =
  <T extends { id?: string }>(list: T[]) =>
  (ctx: HandlerContext) => {
    const id = ctx.path.split("/")[2];
    return list.find((item) => item.id === id) ?? list[0];
  };

const paginated = (data: unknown[]) => ({
  data,
  page: 1,
  page_size: 10,
  total_pages: 1,
  total_records: data.length,
});

export const coreFixtures: Record<string, Handler> = {
  "GET /groups": groups,
  "GET /groups/:id": byId(groups),

  "GET /peers": peers,
  "GET /peers/:id": byId(peers),
  "GET /peers/:id/accessible-peers": (ctx: HandlerContext) =>
    peers.filter((p) => p.id !== ctx.path.split("/")[2]).slice(0, 7),
  "GET /peers/:id/jobs": jobs,
  "GET /peers/edr/bypassed": [{ peer_id: PEER.windows }],

  "GET /users": (ctx: HandlerContext) => {
    const service = ctx.query.get("service_user");
    if (service === null) return users;
    return users.filter((u) => u.is_service_user === (service === "true"));
  },
  "GET /users/:id": byId(users),
  "GET /users/:id/tokens": tokens,
  "GET /users/invites": invites,

  "GET /setup-keys": setupKeys,
  "GET /setup-keys/:id": byId(setupKeys),

  "GET /events/audit": auditEvents,
  "GET /events/network-traffic": paginated(trafficEvents),

  "GET /locations/countries": countries,
  "GET /locations/countries/:code/cities": cities,

  "GET /instance": { setup_required: false },
  "GET /instance/version": {
    management_current_version: "0.60.0",
    management_available_version: "0.60.0",
    dashboard_available_version: "2.20.0",
    management_update_available: false,
  },

  "GET /identity-providers": identityProviders,

  "GET /server/backup": {
    config: {
      s3_endpoint: "https://s3.eu-central-1.amazonaws.com",
      s3_region: "eu-central-1",
      s3_bucket: "netbirdy-backups",
      s3_access_key: "********",
      s3_secret_key: "********",
      s3_prefix: "mgmt/",
      interval: "6h",
      encryption_key: "********",
    },
    status: {
      enabled: true,
      running: false,
      last_run: "2026-10-07T12:00:00Z",
      last_error: "",
      last_snapshot: "mgmt/snapshot-2026-10-07T12-00-00Z.nbbackup",
      next_run: "2026-10-08T12:00:00Z",
    },
  },
  "PUT /server/backup": (ctx: HandlerContext) => ({
    config: ctx.body,
    status: {
      enabled: true,
      running: false,
      last_run: "2026-10-07T12:00:00Z",
      last_error: "",
      last_snapshot: "mgmt/snapshot-2026-10-07T12-00-00Z.nbbackup",
      next_run: "2026-10-08T12:00:00Z",
    },
  }),
  "POST /server/backup/run": {
    key: "mgmt/snapshot-2026-10-08T09-30-00Z.nbbackup",
    size: 2480934,
  },
  "GET /server/backup/snapshots": {
    snapshots: [
      {
        key: "mgmt/snapshot-2026-10-07T12-00-00Z.nbbackup",
        size: 2480934,
        last_modified: "2026-10-07T12:00:00Z",
      },
      {
        key: "mgmt/snapshot-2026-10-06T12-00-00Z.nbbackup",
        size: 2377610,
        last_modified: "2026-10-06T12:00:00Z",
      },
      {
        key: "mgmt/snapshot-2026-10-05T12-00-00Z.nbbackup",
        size: 2290154,
        last_modified: "2026-10-05T12:00:00Z",
      },
    ],
  },
};
