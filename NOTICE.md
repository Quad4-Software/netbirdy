# Notice

NetBirdy is a modified fork of NetBird
(https://github.com/netbirdio/netbird) and of the NetBird dashboard
(https://github.com/netbirdio/dashboard).

Modifications are copyright Sudo-Ivan 2026.

## Summary of changes

- Rebrand of the product name from NetBird to NetBirdy across
  user-facing strings, documentation, and packaging metadata.
- Binary rename: netbird -> netbirdy, netbird-mgmt -> netbirdy-mgmt,
  netbird-signal -> netbirdy-signal, netbird-relay -> netbirdy-relay,
  netbird-server -> netbirdy-server, netbird-upload -> netbirdy-upload.
- New dark and light dashboard theme.
- Podman tooling for local preview under podman/.
- Monorepo layout: the dashboard sources are merged into this
  repository under dashboard/.

## License map

This repository is covered by two licenses:

- BSD-3-Clause: everything except the directories listed below.
- AGPL-3.0-only: management/, signal/, relay/, combined/, and
  dashboard/.

Each directory keeps the LICENSE file of the upstream project it came
from. The full license texts are in the LICENSES/ directory and the
per-directory LICENSE files.

## Change notice and source offer

Per AGPLv3 section 5(a), this file and the repository history document
that the covered works have been modified, and the date of those
modifications. Per AGPLv3 section 13, the complete corresponding
source code for the versions of the covered works distributed through
this project is offered at:

    https://github.com/Sudo-Ivan/netbirdy

## Trademark

NetBird is a trademark of NetBird GmbH. NetBirdy is not affiliated
with, endorsed by, or sponsored by NetBird GmbH.

Nothing in this file is legal advice.

## Fonts

Instrument Sans and JetBrains Mono are vendored under the SIL Open Font License 1.1.
- Instrument Sans: Copyright The Instrument Sans Project Authors (https://github.com/Instrument/instrument-sans)
- JetBrains Mono: Copyright 2020 The JetBrains Project Authors (https://github.com/JetBrains/JetBrainsMono)

Embedded documentation under dashboard/content/docs/upstream/ and
dashboard/public/docs-img/ is adapted from the NetBird documentation
(github.com/netbirdio/docs), BSD-3-Clause, copyright Wiretrustee UG
and AUTHORS, pinned to commit 2382247.
