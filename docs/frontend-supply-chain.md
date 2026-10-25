# Frontend supply chain settings

All frontends (dashboard, proxy/web, client/ui/frontend) install through
the pnpm workspace at the repository root. The settings live in
pnpm-workspace.yaml and apply to every package.

## Settings

- `minimumReleaseAge: 10080` delays any release younger than 7 days,
  matching the fork rule that new dependency versions must be a week old.
  `minimumReleaseAgeStrict: true` fails instead of falling back to a
  newer in-range version when nothing is old enough.
- `blockExoticSubdeps: true` forbids git URLs, tarball URLs and other
  non-registry sources below the root, so a transitive dependency cannot
  pull code that bypasses registry scanning.
- `trustPolicy: no-downgrade` fails an install where a package's trust
  evidence regresses (for example trusted-publisher provenance replaced
  by none). `trustPolicyIgnoreAfter: 525600` exempts releases older than
  a year because provenance did not exist then.
- `strictDepBuilds: true` turns dependency build scripts that are not in
  `allowBuilds` into a hard error. `allowBuilds` lists only packages
  whose install script is actually required (esbuild, sharp,
  @tailwindcss/oxide, unrs-resolver).
- `engineStrict: true` enforces the root engines field (Node >= 24).
- `verifyDepsBeforeRun: error` checks the lockfile before `pnpm run` so
  a stale install surfaces as an error, not a surprise rebuild.
- `shamefullyHoist: false` keeps the strict node_modules layout.

## Approving a build script

Run `pnpm approve-builds`, pick the package, and pnpm writes it to
`allowBuilds`. Review the install script first; approve only what the
package genuinely needs to run.

## Urgent security patch

Exempt one exact version with `minimumReleaseAgeExclude`, for example
`- 'vite@8.3.9'`. Do not lower `minimumReleaseAge` itself.

## Dependabot

Do not combine Dependabot `cooldown` with `minimumReleaseAge`. The
resolver cooldown has no exception for Dependabot's own urgent PRs.
Pick one control; this repo uses the resolver cooldown.

## Lockfile check

scripts/check-frontend-lock.sh runs in CI: it fails when a package-lock.json
or yarn.lock exists outside node_modules, or when
`pnpm install --frozen-lockfile --lockfile-only` would rewrite the lockfile.
