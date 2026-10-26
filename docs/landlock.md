# Landlock sandbox

The server processes restrict their own file system access with
[Landlock](https://landlock.io) as soon as their configuration is loaded. After
that point the process can only touch the paths it needs, so a bug in the
server cannot read or change anything else on the host. It applies to the
combined server, the management, signal and relay servers, the reverse proxy,
the upload server and the updater.

Landlock needs Linux 5.13 or newer with Landlock enabled in the kernel (check
`cat /sys/kernel/security/lsm`). On other systems and kernels the sandbox is
skipped and a log line says so.

## What is allowed

| Access | Paths |
| --- | --- |
| Read and execute | `/etc`, `/usr`, `/lib*`, `/bin`, `/sbin`, `/proc`, `/sys`, `/run`, `/dev` and the directory of the binary |
| Read and write, no execute | the data directory, log files, certificate cache, the update mirror cache, temporary directories |
| Read only | the configuration file, TLS certificate and key files, the shield config |

Everything else is denied: other users' home directories, other services' data,
and executing anything that was written to a writable directory. The paths come
from the configuration, so a path you set in the config is allowed automatically.

Network access is not restricted. The kernel does not apply Landlock bind rules
to multipath TCP, which Go uses by default, and the reverse proxy opens
listeners on ports chosen at run time.

## Settings

| Variable | Values | Effect |
| --- | --- | --- |
| `NB_LANDLOCK` | `best-effort` (default), `strict`, `off` | `strict` stops the server if the kernel cannot enforce the policy, `off` disables the sandbox |
| `NB_LANDLOCK_READS` | `true` (default), `false` | `false` restricts writes only and leaves reads and execution open |
| `NB_LANDLOCK_RO` | paths separated by `:` | extra read-only directories |
| `NB_LANDLOCK_RW` | paths separated by `:` | extra read-write directories |

If a feature fails with a permission error after an upgrade, add the path it
needs with `NB_LANDLOCK_RO` or `NB_LANDLOCK_RW`. Use `NB_LANDLOCK=off` to rule
the sandbox out when debugging.

## The client

The client daemon is not sandboxed by default. It configures network
interfaces, firewall rules and DNS, starts helper programs, and its SSH server
starts shells for users. A policy is inherited by all of them.

You can opt in to a write-only policy for the daemon with
`NB_LANDLOCK_CLIENT=on` in the service environment (`strict` stops the daemon
if the kernel cannot enforce it). Reads and execution stay unrestricted. Writes
are limited to `/etc`, `/var`, `/run`, `/tmp`, `/dev`, `/proc`, `/sys`,
`/home`, `/root`, `/mnt`, `/media` and `/srv`, so the daemon and the helpers it
starts cannot change programs, libraries or boot files under `/usr`, `/bin`,
`/sbin`, `/lib*`, `/boot` and `/opt`. `NB_LANDLOCK_RW` adds more paths.

The SSH server will not start while this sandbox is active, because shells it
starts would inherit the write limits. Leave `NB_LANDLOCK_CLIENT` unset on
peers that accept SSH.
