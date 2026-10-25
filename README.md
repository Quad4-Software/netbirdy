<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/brand/wordmark-dark.svg" width="260">
    <source media="(prefers-color-scheme: light)" srcset="docs/brand/wordmark-light.svg" width="260">
    <img src="docs/brand/wordmark-light.svg" alt="NetBirdy" width="260">
  </picture>
</p>

<p align="center">
  <b>A private network for your devices that you run yourself.</b><br>
  Laptops, servers, phones and home lab machines talk to each other directly and securely, from anywhere.
</p>

<p align="center">
  <a href="https://github.com/Sudo-Ivan/netbirdy/actions/workflows/golang-test-linux.yml"><img src="https://github.com/Sudo-Ivan/netbirdy/actions/workflows/golang-test-linux.yml/badge.svg" alt="CI"></a>
</p>

## What is this?

Imagine all your computers sitting on the same home network, even when one is
in a datacenter, one is at the office and one is in your backpack. That is what
NetBirdy gives you. Each device runs a small program, signs in once, and then
connects straight to your other devices over encrypted WireGuard tunnels. You
decide who can reach what with simple access rules.

NetBirdy is a community fork of [NetBird](https://github.com/netbirdio/netbird).
It keeps everything that makes NetBird work, and adds a nicer control panel,
more protection for your server, and tools for people who run their own
infrastructure.

**Already using NetBird?** You do not have to start over. NetBirdy is a drop-in
replacement. Your devices, keys and settings keep working, and the official
NetBird clients can connect to a NetBirdy server because it speaks the same protocol.

## Why people pick NetBirdy

**A control panel that is pleasant to use**
A clean black dark theme and a warm light theme, a quick search and keyboard
shortcuts (press `?` in the panel to see them), and built-in help pages that
work without internet access.

**Protection built in**
- A web shield for services you publish. It spots scanners and crawlers by how
  they behave and slows them down or blocks them, without bothering real visitors.
- An alarm for suspicious sign-ins. It notices things like a device suddenly
  appearing from a new place or many failed logins. It can just tell you, or it
  can ask for approval before a device is allowed back in.
- Ready-made rules for [Wazuh](https://wazuh.com), if you already run it for
  security monitoring.

**Keeps working when the server does not**
If your server goes offline, devices that are already connected stay connected.
After a restart they can reconnect to their peers using what they remembered,
instead of going dark until the server comes back.

**Easy to run yourself**
- One command starts a complete test setup on your own machine with Podman.
- Signing in with [Pocket ID](https://pocket-id.org) (passkeys) has a guided setup.
- NetBirdy turns off the usage reports that NetBird sends to its own servers, and update checks are
  opt-in. Nothing is sent anywhere unless you turn it on.
- Updates come from signed releases, and the server can update itself with an
  optional helper container that makes a backup first and rolls back if
  something goes wrong.

**Fast, modern web access**
Publish internal web apps through the built-in reverse proxy, with HTTP/3,
WebSockets and streaming support.

## Try it in five minutes

You need [Podman](https://podman.io) and `make`.

```sh
git clone https://github.com/Sudo-Ivan/netbirdy.git
cd netbirdy
make podman-build   # builds the server and the control panel
make podman-up      # creates your settings and starts everything
```

Then open the control panel at **http://localhost:8080**. Your admin email and a
generated password are saved in `podman/.env` on your machine.

When you are done:

```sh
make podman-down    # stop it
make podman-clean   # stop it and remove everything it created
```

More options (other ports, resetting, running the tests) are in
[podman/README.md](podman/README.md).

## Connect your first device

1. In the control panel, open **Setup Keys** and create a key.
2. Install the client on the device. Packages and installers are on the
   [Releases page](https://github.com/Sudo-Ivan/netbirdy/releases). The program is
   called `netbirdy`.
3. Connect it:
   ```sh
   sudo netbirdy up --management-url http://YOUR-SERVER:8081 --setup-key YOUR-KEY
   ```
4. Run `netbirdy status` to see your other devices.

Repeat on every device. Use the **Access Control** page to choose who can talk to whom.

Coming from NetBird? Read [Moving from NetBird](docs/migrating-from-netbird.md).
Installing the NetBirdy package replaces the NetBird one and keeps your existing
settings.

## Questions people ask

**Is this the same as NetBird?**
It is built on NetBird and speaks the same protocol, so the two work together.
NetBirdy adds its own look, extra security features and fork-specific tooling.
It is an independent project and is not affiliated with the NetBird company.

**Can I mix NetBird and NetBirdy?**
Yes. Stock NetBird clients can join a NetBirdy server. Some NetBirdy-only
features, like offline reconnect and update downloads through your server, only
work with the NetBirdy client.

**Does it send data to anyone?**
Not by default. The usage reports to NetBird's servers are off, and update
checking is opt-in. Fonts and help pages are bundled, so the control panel does
not load anything from third-party sites.

**Is it free?**
Yes. See the license section below.

**Where do I report a security problem?**
Privately, as described in [SECURITY.md](SECURITY.md).

## Guides

| I want to | Read |
| --- | --- |
| Move from NetBird | [Moving from NetBird](docs/migrating-from-netbird.md) |
| Understand what happens when the server is down | [Offline mode](docs/offline-mode.md) |
| Get alerts about suspicious sign-ins | [Security monitoring](docs/security-monitoring.md) |
| Send events to Wazuh | [Wazuh](docs/wazuh.md) |
| Sign in with passkeys | [Pocket ID](docs/pocket-id.md) |
| Keep the server and clients up to date | [Updating](docs/updating.md) and [Update mirror](docs/update-mirror.md) |
| Protect published web apps from scanners | [Web shield](proxy/docs/shield.md) |
| Use my own domain for published apps | [Custom domains](docs/custom-domain-validation.md) |
| Run the CI checks on my own server | [Crow CI](docs/ci-crow.md) |
| Maintain a fork or cut a release | [Maintaining](docs/maintaining.md) |

The full list is in [docs/README.md](docs/README.md). The control panel also has
built-in help under **Docs**, so many of these guides are one click away.

## For developers

<details>
<summary>What stays the same as NetBird, and why</summary>

NetBirdy changes names and looks, but keeps the parts that other software
depends on so upgrades never break:

| Kept as in NetBird | Why |
| --- | --- |
| Go module path `github.com/netbirdio/netbird` | existing imports keep compiling |
| `NB_*` environment variables | existing configs keep working |
| `/etc/netbird`, the `netbird` service and user | in-place upgrades |
| Wire protocol, API and setup keys | works with stock clients |
| `X-NetBird-Fwd-*` headers | forwarded-identity protocol |
| Package `Provides`, `Replaces` and `Conflicts: netbird` | package-level migration |
| `docs.netbird.io` and `pkgs.netbird.io` links where there is no fork equivalent | upstream links keep working |

</details>

<details>
<summary>Repository layout</summary>

```
client/         the agent that runs on each device (netbirdy)
client/ui/      desktop app
management/     control plane with REST and gRPC APIs
signal/         helps devices find each other
relay/          carries traffic when devices cannot connect directly
combined/       all-in-one server binary
dashboard/      the control panel (Next.js)
proxy/          reverse proxy with the web shield
idp/            built-in identity provider (Dex)
updater/        optional helper that updates the server stack
shared/         shared API types and update verification
podman/         local stack (Containerfiles, compose, setup)
integrations/   third-party integrations (Wazuh)
```

</details>

Issues and pull requests are welcome at
[github.com/Sudo-Ivan/netbirdy](https://github.com/Sudo-Ivan/netbirdy). Before
pushing, run `tools/rebrand/rebrand.sh --check` for naming changes and
`docs/brand/generate.sh --check` for brand assets.

## License

NetBirdy is open source. Most of the code is BSD-3-Clause. The server pieces
(`management/`, `signal/`, `relay/`, `combined/`) and the control panel
(`dashboard/`) are AGPL-3.0. If you modify and run those parts as a service, you
must offer your users the source. [NOTICE.md](NOTICE.md) and the `LICENSES/`
folder have the full texts, the list of changes from NetBird and the source offer.

## Trademark

NetBird is a trademark of its owner. NetBirdy is an independent fork and is not
affiliated with or endorsed by the upstream project.
