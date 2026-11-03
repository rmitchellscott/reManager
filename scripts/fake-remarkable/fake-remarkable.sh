#!/usr/bin/env bash
# Runs a fake reMarkable in Docker for testing long jobs against a real SSH
# session: it passes reManager's connect checks as a Paper Pro, and each mod it
# installs or removes takes a set number of seconds.
#
#   fake-remarkable.sh start [seconds]   start it, on port 2222 with password "test"
#   fake-remarkable.sh seconds <n>       change the time per mod, even mid-install
#   fake-remarkable.sh fail on|off       make installs fail after the first mod
#   fake-remarkable.sh log               follow the commands reManager sends
#   fake-remarkable.sh stop
set -euo pipefail

cd "$(dirname "$0")"
container=remanager-fake-remarkable
port=${FAKE_PORT:-2222}
password=${FAKE_PASSWORD:-test}

case "${1:-}" in
  start)
    echo "${2:-30}" > step-seconds
    docker run -d --rm --privileged --name "$container" -p "$port:22" \
      -e FAKE_PASSWORD="$password" -v "$PWD:/fake" alpine:3 /fake/container-setup.sh >/dev/null
    until docker logs "$container" 2>&1 | grep -q "Server listening"; do sleep 1; done
    lan_address=$(ipconfig getifaddr en0 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}')
    echo "Connect to ${lan_address:-this machine}:$port with password \"$password\" ($(cat step-seconds) s per mod)."
    echo "From the Android emulator, use 10.0.2.2:$port."
    ;;
  seconds) echo "$2" > step-seconds ;;
  fail) if [ "${2:-}" = on ]; then touch fail-installs; else rm -f fail-installs; fi ;;
  log) docker exec "$container" sh -c 'touch /var/log/remanager-vellum.log; tail -f /var/log/remanager-vellum.log' ;;
  stop) docker stop "$container" >/dev/null ;;
  *) sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'; exit 1 ;;
esac
