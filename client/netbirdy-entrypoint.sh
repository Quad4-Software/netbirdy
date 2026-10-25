#!/usr/bin/env bash
set -eEuo pipefail

: ${NB_ENTRYPOINT_SERVICE_TIMEOUT:="30"}
NETBIRD_BIN="${NETBIRD_BIN:-"netbirdy"}"
export NB_LOG_FILE="${NB_LOG_FILE:-"console,/var/log/netbird/client.log"}"
service_pids=()

_log() {
  # mimic Go logger's output for easier parsing
  # 2025-04-15T21:32:00+08:00 INFO client/internal/config.go:495: setting notifications to disabled by default
  printf "$(date -Isec) ${1} ${BASH_SOURCE[1]}:${BASH_LINENO[1]}: ${2}\n" "${@:3}" >&2
}

info() {
  _log INFO "$@"
}

warn() {
  _log WARN "$@"
}

on_exit() {
  info "Shutting down NetBirdy daemon..."
  if test "${#service_pids[@]}" -gt 0; then
    info "terminating service process IDs: ${service_pids[@]@Q}"
    kill -TERM "${service_pids[@]}" 2>/dev/null || true
    wait "${service_pids[@]}" 2>/dev/null || true
  else
    info "there are no service processes to terminate"
  fi
}

wait_for_daemon_startup() {
  local timeout="${1}"
  if [[ "${timeout}" -eq 0 ]]; then
    info "not waiting for daemon startup due to zero timeout."
    return
  fi

  local deadline=$((SECONDS + timeout))
  while [[ "${SECONDS}" -lt "${deadline}" ]]; do
    if "${NETBIRD_BIN}" status --check live 2>/dev/null; then
      return
    fi
    sleep 1
  done

  warn "daemon did not become responsive after ${timeout} seconds, exiting..."
  exit 1
}

connect() {
  info "running 'netbirdy up'..."
  # The daemon keeps its own retry/offline logic; a failed up (for
  # example while management is down and offline mode takes over) must
  # not stop the container.
  if ! "${NETBIRD_BIN}" up; then
    warn "'netbirdy up' failed; daemon continues to retry in the background"
    return 0
  fi
  return 0
}

main() {
  trap 'on_exit' SIGTERM SIGINT EXIT
  "${NETBIRD_BIN}" service run &
  service_pids+=("$!")
  info "registered new service process 'netbirdy service run', currently running: ${service_pids[@]@Q}"

  wait_for_daemon_startup "${NB_ENTRYPOINT_SERVICE_TIMEOUT}"
  # NB_ENTRYPOINT_SKIP_UP=1 skips the explicit 'netbirdy up': the daemon
  # auto-connects the active profile and 'up' would kill it via Down() if
  # its login wait fails while offline mode is active.
  if [[ "${NB_ENTRYPOINT_SKIP_UP:-0}" != "1" ]]; then
    connect
  fi

  wait "${service_pids[@]}"
}

main "$@"
