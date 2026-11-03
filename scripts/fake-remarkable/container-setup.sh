#!/bin/sh
# Entrypoint of the fake reMarkable container: an sshd that answers the
# checks reManager runs on connect as a Paper Pro. FAKE_OS_VERSION sets the OS
# it reports, and FAKE_VELLUM=off leaves Vellum off for reManager to install.
set -e
command -v sshd >/dev/null || apk add --no-cache openssh openssh-sftp-server bash coreutils openssl umount tzdata >/dev/null

# A reboot restarts the container, which keeps this part as the tablet keeps
# its settings, and redoes the rest, as the tablet loses /run.
if [ ! -e /etc/fake-remarkable ]; then
  touch /etc/fake-remarkable
  # The tablet's sh is bash, and Vellum's bootstrap.sh relies on brace expansion.
  ln -sf /bin/bash /bin/sh
  ssh-keygen -A >/dev/null
  echo "root:${FAKE_PASSWORD}" | chpasswd
  sed -i "s/^#\?PermitRootLogin.*/PermitRootLogin yes/; s/^#\?PasswordAuthentication.*/PasswordAuthentication yes/; s|^Subsystem.*||" /etc/ssh/sshd_config
  printf "ForceCommand /fake/log-command.sh\nSubsystem sftp internal-sftp\n" >> /etc/ssh/sshd_config
  sed -i 's|^root:\([^:]*\):0:0:[^:]*:/root:|root:\1:0:0:root:/home/root:|' /etc/passwd

  mkdir -p /usr/share/remarkable
  version=${FAKE_OS_VERSION:-3.26.0.68}
  printf "[General]\nRELEASE_VERSION=%s\nREMARKABLE_RELEASE_VERSION=%s\n" "$version" "$version" > /usr/share/remarkable/update.conf
  tr -d - < /proc/sys/kernel/random/uuid > /etc/machine-id
  ln -sfn /usr/share/zoneinfo/UTC /etc/localtime
  install -m 755 /fake/systemctl /fake/timedatectl /usr/local/bin/
  install -m 755 /fake/xochitl /usr/bin/xochitl
  # Package scripts run with apk's PATH, which leaves out /usr/local/bin.
  ln -sf /usr/local/bin/systemctl /usr/local/bin/timedatectl /usr/bin/
  # launcherctl's xovi launcher copies the unit into a mount of its own.
  mkdir -p /usr/lib/systemd/system/xochitl.service.d
  printf '[Service]\nExecStart=/usr/bin/xochitl\n' > /usr/lib/systemd/system/xochitl.service

  mkdir -p /home/root/.config/remarkable /home/root/.local/share/remarkable/xochitl
  printf "[General]\n" > /home/root/.config/remarkable/xochitl.conf

  if [ "${FAKE_VELLUM:-on}" = on ]; then
    mkdir -p /home/root/.vellum/bin
    ln -sf /fake/vellum /home/root/.vellum/bin/vellum
    /home/root/.vellum/bin/vellum info >/dev/null

    # Hashtable rebuild, for jobs that report no progress and end in a prompt. It
    # takes about as long as the real one, and starting the UI afterward succeeds.
    echo "qt-resource-rebuilder 1.0.0-r0" >> /home/root/.vellum/state/fake-installed
    /fake/write-hashtab /home/root/xovi/exthome/qt-resource-rebuilder/hashtab
    printf '#!/bin/sh\necho "Rebuilding hashtable..."\nsleep "$(cat /fake/hashtab-seconds 2>/dev/null || echo 65)"\n/fake/write-hashtab /home/root/xovi/exthome/qt-resource-rebuilder/hashtab\necho "Done."\n' > /home/root/xovi/rebuild_hashtable
    printf '#!/bin/sh\nexit 0\n' > /home/root/xovi/start
    chmod +x /home/root/xovi/rebuild_hashtable /home/root/xovi/start
  fi
fi

# Device detection reads the model from sysfs, which a container cannot write.
mount -t tmpfs tmpfs /sys/devices
mkdir -p /sys/devices/soc0
echo "reMarkable Ferrari" > /sys/devices/soc0/machine

# Keep-awake finds the pen by its BTN_STYLUS capability and writes input events
# to it, which land in a plain file here.
mount -t tmpfs tmpfs /sys/class/input
mkdir -p /sys/class/input/event1/device/capabilities /dev/input
echo "c03 0 0 0 0 0" > /sys/class/input/event1/device/capabilities/key
: > /dev/input/event1

mount -t tmpfs tmpfs /run
# Volatile on the tablet too, so they stay writable while reManager has the
# root remounted read-only.
mount -t tmpfs tmpfs /var/log
mount -t tmpfs tmpfs /tmp
mkdir -p /run/fake-systemd
touch /run/fake-systemd/update-engine.enabled /run/fake-systemd/update-engine.active
/usr/local/bin/systemctl start xochitl

exec /usr/sbin/sshd -D -e
