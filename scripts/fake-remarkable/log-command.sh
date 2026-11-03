#!/bin/sh
# Records every command reManager runs, then runs it. The SFTP subsystem
# comes through here too, because ForceCommand also replaces subsystems.
# While the root is read-only the record is skipped, since the shell's
# complaint would land in the command's output.
printf '%s\t%s\n' "$(date +%T)" "${SSH_ORIGINAL_COMMAND:-<interactive>}" 2>/dev/null >> /var/log/remanager-commands.log
case "$SSH_ORIGINAL_COMMAND" in
  internal-sftp) exec /usr/lib/ssh/sftp-server ;;
  "") exec /bin/bash -l ;;
  *) exec /bin/bash -c "$SSH_ORIGINAL_COMMAND" ;;
esac
