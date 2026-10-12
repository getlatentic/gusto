package vmwall

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Env is the computer an install is for. A test is any computer it describes.
type Env struct {
	Platform Platform
	// Getenv reads the person's environment.
	Getenv func(string) string
	// Exists says whether a path is there.
	Exists func(path string) bool
	// Get downloads a file.
	Get Download
	// GetAsset downloads a file attached to a release of haas.
	GetAsset func(ctx context.Context, release, name string, dst io.Writer) error
	// Release is the release of haas this is, or empty for a build that is not one.
	Release string
	// Run runs a program with exactly the environment given and returns what it printed.
	Run func(ctx context.Context, env []string, program string, args ...string) (string, error)
	// Start keeps the gateway running, now and at login.
	Start func(Layout) error
	// Say is where progress is written.
	Say io.Writer
}

// Plan is an install that has been checked will work, and says what it will download.
type Plan struct {
	Env
	Layout
	Programs []Program
	// Image is the checksum and size of haas's root file system, which the release names, when the person names none.
	Image Rootfs
	// ImageFile is the person's own root file system, when they name one. Without one, the install takes the image of haas's own release.
	ImageFile string
	// ConfigHome is where the openshell command keeps its gateways: XDG_CONFIG_HOME, or the .config folder of the person's home.
	ConfigHome string
}

// Rootfs is a root file system as a release names it.
type Rootfs struct {
	SHA256 string
	Size   int64
}

// Prepare checks that this computer can run virtual machines and that the install has everything it needs, before anything is
// downloaded. imageFile is a root file system the person already has, or empty for the one haas publishes.
func Prepare(ctx context.Context, env Env, haasHome, user, imageFile string) (Plan, error) {
	files, err := ProgramsFor(env.Platform)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{Env: env, Layout: Layout{Root: filepath.Join(haasHome, "openshell"), User: user}, Programs: files, ImageFile: imageFile}
	if err := plan.hardware(); err != nil {
		return Plan{}, err
	}
	if err := plan.diskTools(); err != nil {
		return Plan{}, err
	}
	switch {
	case imageFile != "" && !env.Exists(imageFile):
		return Plan{}, fmt.Errorf("%s is not there", imageFile)
	case imageFile == "" && env.Release == "":
		return Plan{}, errors.New("this haas was not built from a release, so it has no root file system to install: name one with --image FILE")
	}
	if imageFile == "" {
		if plan.Image, err = plan.releasedImage(ctx); err != nil {
			return Plan{}, err
		}
	}
	plan.ConfigHome = env.Getenv("XDG_CONFIG_HOME")
	if plan.ConfigHome == "" {
		if imageFile == "" {
			if plan.Image, err = plan.releasedImage(ctx); err != nil {
				return Plan{}, err
			}
		}
		plan.ConfigHome = filepath.Join(user, ".config")
	}
	return plan, nil
}

// releasedImage reads what the release says of its root file system: a line of its checksum file, which holds the checksum and the
// size in bytes. The size is there so the person can be told it before they agree to the download.
func (p Plan) releasedImage(ctx context.Context) (Rootfs, error) {
	var listed bytes.Buffer
	if err := p.GetAsset(ctx, p.Release, RootfsAsset(p.Platform.GOARCH)+".sha256", &listed); err != nil {
		return Rootfs{}, fmt.Errorf("haas %s has no root file system for %s: %w", p.Release, p.Platform.GOARCH, err)
	}
	fields := strings.Fields(listed.String())
	if len(fields) < 2 {
		return Rootfs{}, fmt.Errorf("the checksum file of haas %s's root file system is not a checksum and a size", p.Release)
	}
	size, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || size <= 0 {
		return Rootfs{}, fmt.Errorf("the checksum file of haas %s's root file system has no size", p.Release)
	}
	return Rootfs{SHA256: fields[0], Size: size}, nil
}

func (p Plan) hardware() error {
	switch p.Platform.GOOS {
	case "linux":
		if !p.Exists("/dev/kvm") {
			return errors.New("this computer has no /dev/kvm, which virtual machines need")
		}
	case "darwin":
		if out, err := p.Run(context.Background(), nil, "sysctl", "-n", "kern.hv_support"); err != nil || strings.TrimSpace(out) != "1" {
			return errors.New("this Mac does not offer hardware virtualization")
		}
	}
	return nil
}

// diskTools checks the programs that build a sandbox's disk are where the gateway will look.
func (p Plan) diskTools() error {
	folders := append([]string{p.Bin()}, toolFolders...)
	folders = append(folders, filepath.SplitList(p.Getenv("PATH"))...)
	for _, tool := range diskTools {
		found := false
		for _, folder := range folders {
			if p.Exists(filepath.Join(folder, tool)) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s is not on this computer, and the virtual machine wall needs it to build each job's disk: %s", tool, p.howToGetTools())
		}
	}
	return nil
}

func (p Plan) howToGetTools() string {
	if p.Platform.GOOS == "darwin" {
		return "brew install e2fsprogs"
	}
	return "install your system's e2fsprogs package"
}

// Words says what the install does, for the person to agree to.
func (p Plan) Words() string {
	var b strings.Builder
	fmt.Fprintf(&b, "haas will download OpenShell %s (%d MB) from github.com/NVIDIA/OpenShell", Version, programBytes(p.Programs)>>20)
	if p.ImageFile == "" {
		fmt.Fprintf(&b, " and, from haas's release %s, the Linux image the agents run in (%d MB)", p.Release, p.Image.Size>>20)
	}
	fmt.Fprintf(&b, ".\nIt checks every file against a checksum haas carries, keeps them in %s, and keeps a gateway running for you", p.Root)
	b.WriteString(map[string]string{"darwin": " as a launch agent", "linux": " as a systemd user service"}[p.Platform.GOOS])
	b.WriteString(".\nEach job then runs in a virtual machine and sees only its own folder. It needs no password and changes nothing outside your own folders.")
	return b.String()
}

func programBytes(programs []Program) int64 {
	var total int64
	for _, program := range programs {
		total += program.Size
	}
	return total
}

func (p Plan) say(format string, args ...any) { fmt.Fprintf(p.Say, format+"\n", args...) }

func ensureDirs(dirs ...string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}
