#!/bin/sh

# Step 1, decide if we should use systemd or init/upstart
use_systemctl="True"
systemd_version=0
if ! command -V systemctl >/dev/null 2>&1; then
  use_systemctl="False"
else
    systemd_version=$(systemctl --version | head -1 | sed 's/systemd //g')
fi

# The service environment file may hold the PKCS#11 token PIN (NB_TPM_PIN). Package
# upgrades keep a locally modified copy with its old mode, so restrict it on every run.
restrict_env_file() {
    if [ -f /etc/sysconfig/netbirdy ]; then
      chmod 0600 /etc/sysconfig/netbirdy
    fi
    return 0
}

# Remove a legacy NetBird service before installing ours. Config and state in
# /etc/netbird and /var/lib/netbird stay untouched. Idempotent.
migrate_netbird_service() {
    if [ "${use_systemctl}" = "True" ]; then
        if systemctl list-unit-files netbird.service >/dev/null 2>&1; then
            printf "\033[33m Found existing netbird service, migrating to netbirdy\033[0m\n"
            systemctl stop netbird 2> /dev/null || true
            systemctl disable netbird 2> /dev/null || true
        fi
        for unit in /etc/systemd/system/netbird.service /lib/systemd/system/netbird.service /usr/lib/systemd/system/netbird.service; do
            [ -f "$unit" ] && rm -f "$unit"
        done
        systemctl daemon-reload 2> /dev/null || true
    fi
    if [ -x /usr/bin/netbird ]; then
        /usr/bin/netbird service uninstall 2> /dev/null || true
        rm -f /usr/bin/netbird
    fi
    return 0
}

cleanInstall() {
    migrate_netbird_service
    printf "\033[32m Post Install of a clean install\033[0m\n"
    restrict_env_file
    # Step 3 (clean install), enable the service in the proper way for this platform
    /usr/bin/netbirdy service install
    /usr/bin/netbirdy service start
}

upgrade() {
    migrate_netbird_service
    printf "\033[32m Post Install of an upgrade\033[0m\n"
    restrict_env_file
    if [ "${use_systemctl}" = "True" ]; then
      printf "\033[32m Stopping the service\033[0m\n"
      systemctl stop netbirdy 2> /dev/null || true
    fi
    if [ -e /lib/systemd/system/netbirdy.service ]; then
      rm -f /lib/systemd/system/netbirdy.service
      systemctl daemon-reload
    fi
    # will trow an error until everyone upgrade
    /usr/bin/netbirdy service uninstall 2> /dev/null || true
    /usr/bin/netbirdy service install
    /usr/bin/netbirdy service start
}

# Check if this is a clean install or an upgrade
action="$1"
if  [ "$1" = "configure" ] && [ -z "$2" ]; then
  # Alpine linux does not pass args, and deb passes $1=configure
  action="install"
elif [ "$1" = "configure" ] && [ -n "$2" ]; then
    # deb passes $1=configure $2=<current version>
    action="upgrade"
fi

case "$action" in
  "1" | "install")
    cleanInstall
    ;;
  "2" | "upgrade")
    upgrade
    ;;
  *)
    cleanInstall
    ;;
esac