package executor

// rm1 and rm2 give non-interactive SSH commands dropbear's compiled default PATH, which omits the Vellum bin directory, so every device resolves commands the same way.
const devicePath = "/home/root/.vellum/bin:/usr/local/bin:/usr/bin:/bin:/usr/local/sbin:/usr/sbin:/sbin"

func WithDevicePath(cmd string) string {
	return "export PATH=" + devicePath + ":$PATH; " + cmd
}
