// Package vmwall puts OpenShell on a person's computer so that each job runs in a virtual machine: it downloads the programs and
// checks them, signs the driver where the system requires it, makes the certificates and the gateway's configuration, and keeps
// the gateway running. It asks nothing of the person but one agreement to the download, and needs no password.
package vmwall

import "fmt"

// Version is the OpenShell release haas installs. Every file is pinned by checksum, so a release moves only by a change here.
const Version = "v0.1.3"

const releaseURL = "https://github.com/NVIDIA/OpenShell/releases/download/" + Version + "/"

// Program is one of the files of the release.
type Program struct {
	// Name is the program as it is run, and the one file inside its archive.
	Name string
	// Archive is the file of the release that holds it.
	Archive string
	SHA256  string
	Size    int64
}

// URL is where the archive is downloaded from.
func (p Program) URL() string { return releaseURL + p.Archive }

// Platform is an operating system and processor OpenShell is released for.
type Platform struct{ GOOS, GOARCH string }

func (p Platform) String() string { return p.GOOS + "/" + p.GOARCH }

// CLI, Gateway and Driver are the three programs a computer needs.
const (
	CLI     = "openshell"
	Gateway = "openshell-gateway"
	Driver  = "openshell-driver-vm"
)

var programs = map[Platform][]Program{
	{"darwin", "arm64"}: {
		{CLI, "openshell-aarch64-apple-darwin.tar.gz", "2eda3676aa85ff60551b403847090942e119c89f37c84d1635ac166c6b879826", 9450808},
		{Gateway, "openshell-gateway-aarch64-apple-darwin.tar.gz", "a0cff0699f4a608a644805dbbf898c42caa59856274ff3cc2cdd6b3af4bacabe", 33431517},
		{Driver, "openshell-driver-vm-aarch64-apple-darwin.tar.gz", "56e321f990741ca5585f5229e37c178f541b0c38029351fe59b72a36c1c750d4", 30216186},
	},
	{"linux", "amd64"}: {
		{CLI, "openshell-x86_64-unknown-linux-musl.tar.gz", "ed08ea7874c47f22d6e5dcc4b27d9ef1ef5d2060f83035b35ff725d5160830a3", 10349950},
		{Gateway, "openshell-gateway-x86_64-unknown-linux-gnu.tar.gz", "da8dce943fe01cce90243b2bf2587c71a243e4d641a4b616528829b681ced198", 41545155},
		{Driver, "openshell-driver-vm-x86_64-unknown-linux-gnu.tar.gz", "22593640c6179eb87413ab099832fbd133d3a6a4979904d27679e7da6d80a0cb", 31823714},
	},
	{"linux", "arm64"}: {
		{CLI, "openshell-aarch64-unknown-linux-musl.tar.gz", "07687dacefc52bc73b01943f5b58f7a5082c1ceaa228461b9a2a78023b8057c0", 9533199},
		{Gateway, "openshell-gateway-aarch64-unknown-linux-gnu.tar.gz", "801214322227f64f2d3faa6989daaa5e6cd9ef1ab768b5860496deeb1158c27b", 40065083},
		{Driver, "openshell-driver-vm-aarch64-unknown-linux-gnu.tar.gz", "4426ee0992989060eb5987ad7072feb3eb6aecf5a628ad0284c3d7ad5f39974b", 30599726},
	},
}

// RootfsAsset is the file of a haas release that holds the root file system a job's virtual machine starts from: the runner image,
// flattened, for the processor the machine has. A file with the same name and .sha256 added holds its checksum.
func RootfsAsset(goarch string) string { return "haas-rootfs-" + goarch + ".tar.gz" }

// ProgramsFor is the files OpenShell needs on a platform.
func ProgramsFor(p Platform) ([]Program, error) {
	files, ok := programs[p]
	if !ok {
		return nil, fmt.Errorf("OpenShell is not released for %s: it has Apple Silicon Macs and Linux on x86_64 or arm64", p)
	}
	return files, nil
}

// ToolsAsset is the file of a haas release that holds the disk tools (mke2fs, debugfs and e2fsck) built to stand alone, for a Mac,
// which has none of its own. A file with the same name and .sha256 added holds its checksum and size.
const ToolsAsset = "haas-e2fsprogs-darwin-arm64.tar.gz"
