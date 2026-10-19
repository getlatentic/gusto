package vmwall

import (
	"path/filepath"
	"strings"
)

// GatewayName is what gusto calls its gateway to the openshell command, so a gateway the person already has is left alone.
const GatewayName = "gusto"

// gatewayAddress is where the gateway listens, on the loopback.
const gatewayAddress = "https://localhost:17670"

// Layout is where everything gusto installs for the virtual machine wall is kept.
type Layout struct {
	// Root is the folder of OpenShell inside gusto's own, and User the person's home.
	Root, User string
}

func (l Layout) Bin() string      { return filepath.Join(l.Root, "bin") }
func (l Layout) Libexec() string  { return filepath.Join(l.Root, "libexec") }
func (l Layout) TLS() string      { return filepath.Join(l.Root, "tls") }
func (l Layout) Config() string   { return filepath.Join(l.Root, "gateway.toml") }
func (l Layout) Log() string      { return filepath.Join(l.Root, "gateway.log") }
func (l Layout) Settings() string { return filepath.Join(l.Root, "vm.json") }
func (l Layout) Rootfs() string   { return filepath.Join(l.Root, "rootfs") }

// State is the gateway's working folder. The driver's socket lives under it and a socket's path may not pass 104 bytes, so it is
// short, where gusto's own folder on a Mac is not.
func (l Layout) State() string { return filepath.Join(l.User, ".gusto-vm") }

// Program is the path a program of the release is run from. The gateway finds its driver in the libexec folder beside its bin.
func (l Layout) Program(name string) string {
	if name == Driver {
		return filepath.Join(l.Libexec(), name)
	}
	return filepath.Join(l.Bin(), name)
}

// toolFolders are where a computer keeps the programs that build a sandbox's disk (mke2fs, debugfs, e2fsck) when it has them: a
// Homebrew install of e2fsprogs is not on the PATH a service starts with.
var toolFolders = []string{"/opt/homebrew/opt/e2fsprogs/bin", "/opt/homebrew/opt/e2fsprogs/sbin", "/usr/local/opt/e2fsprogs/bin", "/usr/local/opt/e2fsprogs/sbin", "/usr/sbin", "/sbin"}

// diskTools are the programs the driver runs on the host to make each sandbox's disk.
var diskTools = []string{"mke2fs", "debugfs", "e2fsck"}

// GatewayPath is the PATH the gateway runs with: gusto's own folder first, so the disk tools gusto ships are the ones used.
func (l Layout) GatewayPath() string {
	return strings.Join(append([]string{l.Bin()}, append(toolFolders, "/usr/bin", "/bin")...), string(filepath.ListSeparator))
}

// GatewayEnv is the environment the gateway runs with, and nothing more.
func (l Layout) GatewayEnv() []string {
	return []string{
		"OPENSHELL_GATEWAY_CONFIG=" + l.Config(),
		"OPENSHELL_TLS_CERT=" + filepath.Join(l.TLS(), "server", "tls.crt"),
		"OPENSHELL_TLS_KEY=" + filepath.Join(l.TLS(), "server", "tls.key"),
		"OPENSHELL_TLS_CLIENT_CA=" + filepath.Join(l.TLS(), "ca.crt"),
		"OPENSHELL_LOCAL_TLS_DIR=" + l.TLS(),
		"XDG_STATE_HOME=" + l.State(),
		"HOME=" + l.User,
		"PATH=" + l.GatewayPath(),
	}
}

// ConfigText is the gateway's configuration: virtual machines for sandboxes, and the certificate authority a guest trusts.
func (l Layout) ConfigText() string {
	return "[openshell]\nversion = 2\n\n[openshell.gateway]\ncompute_driver = \"vm\"\nguest_tls_ca = " + quote(filepath.Join(l.TLS(), "ca.crt")) +
		"\n\n[openshell.drivers.vm]\nallow_driver_config = true\n"
}

func quote(text string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(text) + `"`
}
