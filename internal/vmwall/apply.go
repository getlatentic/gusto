package vmwall

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// entitlements lets a program use the hypervisor, which macOS refuses to one without it. Signing with no certificate, as the
// program's own formula does, is enough for a program the person runs themselves.
const entitlements = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>com.apple.security.hypervisor</key><true/></dict></plist>
`

// startupWait is how long a gateway has to answer after it starts.
var startupWait = 45 * time.Second

// Apply installs: it downloads and checks the programs, makes the gateway's certificates and configuration, starts the gateway for
// good, and last records what it made so gusto uses it. Run again, it replaces the programs and keeps the certificates.
func (p Plan) Apply(ctx context.Context) error {
	steps := []struct {
		say string
		do  func(context.Context) error
	}{
		{"Downloading OpenShell", p.install},
		{"Getting the disk tools", p.fetchTools},
		{"Signing the virtual machine driver", p.sign},
		{"Making the gateway's certificates", p.certificates},
		{"Configuring the gateway", p.configure},
		{"Getting the agents' image", p.fetchImage},
		{"Starting the gateway", p.start},
		{"Registering the gateway", p.register},
		{"Waiting for the gateway to answer", p.answers},
	}
	if err := ensureDirs(p.Root, p.Bin(), p.Libexec(), p.TLS(), p.Rootfs(), p.State()); err != nil {
		return err
	}
	for _, step := range steps {
		p.say("%s...", step.say)
		if err := step.do(ctx); err != nil {
			return fmt.Errorf("%s: %w", step.say, err)
		}
	}
	return Settings{Version: Version, Binary: p.Program(CLI), Gateway: GatewayName, Image: p.imagePath()}.save(p.Settings())
}

func (p Plan) install(ctx context.Context) error {
	for _, program := range p.Programs {
		archive, err := fetch(ctx, p.Get, program.URL(), program.SHA256, p.Root)
		if err != nil {
			return err
		}
		err = unpackOne(archive, p.Program(program.Name))
		os.Remove(archive)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p Plan) sign(ctx context.Context) error {
	if p.Platform.GOOS != "darwin" {
		return nil
	}
	file, err := os.CreateTemp(p.Root, "entitlements-*.plist")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(entitlements); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_, err = p.Run(ctx, p.cliEnv(), "codesign", "--force", "-s", "-", "--entitlements", file.Name(), p.Program(Driver))
	return err
}

func (p Plan) certificates(ctx context.Context) error {
	if p.Exists(filepath.Join(p.TLS(), "ca.crt")) {
		return nil
	}
	_, err := p.Run(ctx, p.cliEnv(), p.Program(Gateway), "generate-certs", "--output-dir", p.TLS(), "--server-san", "host.openshell.internal")
	return err
}

func (p Plan) configure(context.Context) error {
	return os.WriteFile(p.Config(), []byte(p.ConfigText()), 0o600)
}

// imagePath is the root file system jobs start from, once it is where gusto keeps it.
func (p Plan) imagePath() string {
	if p.ImageFile != "" {
		abs, err := filepath.Abs(p.ImageFile)
		if err != nil {
			return p.ImageFile
		}
		return abs
	}
	return filepath.Join(p.Rootfs(), RootfsAsset(p.Platform.GOARCH))
}

// fetchTools downloads the disk tools into the folder the gateway looks in first, and their licence and source offer beside them.
func (p Plan) fetchTools(ctx context.Context) error {
	if !p.FetchTools {
		return nil
	}
	archive, err := fetch(ctx, func(ctx context.Context, _ string, dst io.Writer) error {
		return p.GetAsset(ctx, p.Release, ToolsAsset, dst)
	}, ToolsAsset, p.Tools.SHA256, p.Root)
	if err != nil {
		return err
	}
	defer os.Remove(archive)
	return unpackInto(archive, func(name string) string {
		if slices.Contains(diskTools, name) {
			return filepath.Join(p.Bin(), name)
		}
		return filepath.Join(p.Root, "licenses", name)
	})
}

// fetchImage downloads gusto's root file system and keeps it only if it is the one the release's checksum file names.
func (p Plan) fetchImage(ctx context.Context) error {
	if p.ImageFile != "" || p.Exists(p.imagePath()) {
		return nil
	}
	name := RootfsAsset(p.Platform.GOARCH)
	downloaded, err := fetch(ctx, func(ctx context.Context, _ string, dst io.Writer) error { return p.GetAsset(ctx, p.Release, name, dst) }, name, p.Image.SHA256, p.Rootfs())
	if err != nil {
		return err
	}
	return os.Rename(downloaded, p.imagePath())
}

func (p Plan) start(context.Context) error { return p.Start(p.Layout) }

// gatewayRecord is the folder the openshell command keeps what it knows of the gateway in.
func (p Plan) gatewayRecord() string {
	return filepath.Join(p.ConfigHome, "openshell", "gateways", GatewayName)
}

// register gives the openshell command the client certificates where it looks for them, which it requires before it will take
// the gateway, and tells it of the gateway under gusto's own name.
func (p Plan) register(ctx context.Context) error {
	mtls := filepath.Join(p.gatewayRecord(), "mtls")
	if err := ensureDirs(mtls); err != nil {
		return err
	}
	for from, to := range map[string]string{"ca.crt": "ca.crt", "client/tls.crt": "tls.crt", "client/tls.key": "tls.key"} {
		text, err := os.ReadFile(filepath.Join(p.TLS(), from))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(mtls, to), text, 0o600); err != nil {
			return err
		}
	}
	_, err := p.Run(ctx, p.cliEnv(), p.Program(CLI), "gateway", "add", gatewayAddress, "--local", "--name", GatewayName)
	return err
}

func (p Plan) answers(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, startupWait)
	defer cancel()
	var last error
	for {
		if _, last = p.Run(ctx, p.cliEnv(), p.Program(CLI), "--gateway", GatewayName, "sandbox", "list"); last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("no answer from the gateway (see %s): %w", p.Log(), last)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// cliEnv is what the programs gusto runs while installing get: the person's home and the folders they search, and nothing else.
func (p Plan) cliEnv() []string {
	env := []string{"HOME=" + p.User, "PATH=" + p.GatewayPath()}
	if config := p.Getenv("XDG_CONFIG_HOME"); config != "" {
		env = append(env, "XDG_CONFIG_HOME="+config)
	}
	return env
}
