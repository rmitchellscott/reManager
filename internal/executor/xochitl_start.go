package executor

// xochitl.service allows 4 starts in 10 minutes; on the Paper Pro line the 5th fails the unit and its OnFailure reboots the tablet, so a user-requested start clears the count first.
func WithXochitlStartReset(cmd string) string {
	return "systemctl reset-failed xochitl 2>/dev/null; " + cmd
}

// A plain restart brings xochitl back without xovi, so a xochitl running xovi is restarted through xovi's own start script.
const RestartXochitlKeepingMods = `if tr '\0' '\n' < /proc/$(pidof xochitl)/environ 2>/dev/null | grep -q 'LD_PRELOAD.*xovi'; then /home/root/xovi/start; else systemctl restart xochitl; fi`
