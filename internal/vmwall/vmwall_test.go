package vmwall

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// archiveOf is a .tar.gz holding one program, and its checksum.
func archiveOf(t *testing.T, name, body string) ([]byte, string) {
	t.Helper()
	var out bytes.Buffer
	zipped := gzip.NewWriter(&out)
	writer := tar.NewWriter(zipped)
	if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	writer.Write([]byte(body))
	writer.Close()
	zipped.Close()
	sum := sha256.Sum256(out.Bytes())
	return out.Bytes(), hex.EncodeToString(sum[:])
}

// aComputer is a Mac that offers virtual machines, with the disk tools, whose downloads are the programs of a made-up release.
type aComputer struct {
	t       *testing.T
	root    string
	user    string
	served  map[string][]byte
	assets  map[string][]byte
	ran     []string
	started bool
	failRun string
}

func newComputer(t *testing.T, arch string) (*aComputer, Env) {
	t.Helper()
	c := &aComputer{t: t, root: t.TempDir(), user: t.TempDir(), served: map[string][]byte{}, assets: map[string][]byte{}}
	platform := Platform{"darwin", arch}
	var files []Program
	for _, name := range []string{CLI, Gateway, Driver} {
		archive, sum := archiveOf(t, name, "#!/bin/sh\necho "+name+"\n")
		c.served[releaseURL+name+".tar.gz"] = archive
		files = append(files, Program{name, name + ".tar.gz", sum, int64(len(archive))})
	}
	was := programs[platform]
	programs[platform] = files
	t.Cleanup(func() {
		if was == nil {
			delete(programs, platform)
		} else {
			programs[platform] = was
		}
	})
	image, imageSum := archiveOf(t, "rootfs.tar", "image")
	c.assets[RootfsAsset(arch)] = image
	c.assets[RootfsAsset(arch)+".sha256"] = []byte(imageSum + "  " + strconv.Itoa(len(image)) + "  " + RootfsAsset(arch) + "\n")
	return c, Env{
		Platform: platform,
		Getenv:   func(string) string { return "" },
		Exists: func(path string) bool {
			if strings.Contains(path, "e2fsprogs") {
				return true
			}
			_, err := os.Stat(path)
			return err == nil
		},
		Get: func(_ context.Context, url string, dst io.Writer) error {
			body, ok := c.served[url]
			if !ok {
				return errors.New("no such file: " + url)
			}
			_, err := dst.Write(body)
			return err
		},
		GetAsset: func(_ context.Context, release, name string, dst io.Writer) error {
			body, ok := c.assets[name]
			if !ok || release != "v9" {
				return errors.New("no such asset: " + release + " " + name)
			}
			_, err := dst.Write(body)
			return err
		},
		Release: "v9",
		Run: func(_ context.Context, _ []string, program string, args ...string) (string, error) {
			line := filepath.Base(program) + " " + strings.Join(args, " ")
			c.ran = append(c.ran, line)
			if c.failRun != "" && strings.Contains(line, c.failRun) {
				return "", errors.New("refused")
			}
			switch {
			case strings.HasPrefix(line, "sysctl"):
				return "1\n", nil
			case strings.HasPrefix(line, "openshell-gateway generate-certs"):
				return "", writeCerts(args[2])
			case strings.HasPrefix(line, "openshell gateway add"):
				if _, err := os.Stat(filepath.Join(c.user, ".config", "openshell", "gateways", GatewayName, "mtls", "tls.key")); err != nil {
					return "", errors.New("mTLS certificates were not found")
				}
			}
			return "", nil
		},
		Start: func(Layout) error { c.started = true; return nil },
		Say:   io.Discard,
	}
}

func writeCerts(dir string) error {
	for _, name := range []string{"ca.crt", "server/tls.crt", "server/tls.key", "client/tls.crt", "client/tls.key"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func TestAnInstallPutsEachProgramWhereTheGatewayFindsItAndRecordsItLast(t *testing.T) {
	c, env := newComputer(t, "arm64")
	plan, err := Prepare(context.Background(), env, c.root, c.user, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{plan.Program(CLI), plan.Program(Gateway), filepath.Join(plan.Libexec(), Driver)} {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o100 == 0 {
			t.Errorf("%s is not an executable program: %v", path, err)
		}
	}
	if !c.started {
		t.Error("the gateway was not started")
	}
	settings, ok := Installed(plan.Layout)
	if !ok || settings.Binary != plan.Program(CLI) || settings.Gateway != GatewayName || !strings.HasSuffix(settings.Image, "haas-rootfs-arm64.tar.gz") {
		t.Fatalf("settings are %+v", settings)
	}
	if _, err := os.Stat(settings.Image); err != nil {
		t.Errorf("the image is not where the settings say: %v", err)
	}
	mtls := filepath.Join(c.user, ".config", "openshell", "gateways", GatewayName, "mtls")
	for _, name := range []string{"ca.crt", "tls.crt", "tls.key"} {
		if _, err := os.Stat(filepath.Join(mtls, name)); err != nil {
			t.Errorf("the openshell command has no %s: %v", name, err)
		}
	}
	signed := false
	for _, line := range c.ran {
		signed = signed || strings.HasPrefix(line, "codesign --force -s - --entitlements")
	}
	if !signed {
		t.Errorf("the driver was not signed on a Mac: %v", c.ran)
	}
}

func TestAnInstallThatFailsLeavesNothingThatHaasWouldUse(t *testing.T) {
	c, env := newComputer(t, "arm64")
	c.failRun = "sandbox list"
	was := startupWait
	startupWait = 50 * time.Millisecond
	t.Cleanup(func() { startupWait = was })
	env.Start = func(Layout) error { return nil }
	plan, err := Prepare(context.Background(), env, c.root, c.user, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "Waiting for the gateway") {
		t.Fatalf("the install did not say where it stopped: %v", err)
	}
	if _, ok := Installed(plan.Layout); ok {
		t.Error("an install that did not finish is used")
	}
}

func TestAFileThatIsNotTheOneHaasCarriesTheChecksumOfIsNotKept(t *testing.T) {
	c, env := newComputer(t, "arm64")
	for url := range c.served {
		if strings.HasPrefix(url, releaseURL) {
			c.served[url] = append(c.served[url], 'x')
		}
	}
	plan, err := Prepare(context.Background(), env, c.root, c.user, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "checksum differs") {
		t.Fatalf("a changed file was accepted: %v", err)
	}
	if _, err := os.Stat(plan.Program(CLI)); err == nil {
		t.Error("a program that failed its checksum is where it could be run")
	}
	if left, _ := filepath.Glob(filepath.Join(plan.Root, "download-*")); len(left) != 0 {
		t.Errorf("a failed download is left behind: %v", left)
	}
}

func TestAComputerThatCannotRunTheWallIsToldBeforeAnythingIsDownloaded(t *testing.T) {
	cases := map[string]func(*aComputer, *Env){
		"an Intel Mac": func(_ *aComputer, e *Env) { e.Platform = Platform{"darwin", "amd64"} },
		"no hypervisor": func(_ *aComputer, e *Env) {
			e.Run = func(context.Context, []string, string, ...string) (string, error) { return "0", nil }
		},
		"no disk tools":    func(_ *aComputer, e *Env) { e.Exists = func(string) bool { return false } },
		"no image for arm": func(c *aComputer, _ *Env) { delete(c.assets, RootfsAsset("arm64")+".sha256") },
		"not a release":    func(_ *aComputer, e *Env) { e.Release = "" },
	}
	for name, spoil := range cases {
		c, env := newComputer(t, "arm64")
		spoil(c, &env)
		downloaded := false
		env.Get = func(context.Context, string, io.Writer) error { downloaded = true; return nil }
		if _, err := Prepare(context.Background(), env, c.root, c.user, ""); err == nil || downloaded {
			t.Errorf("%s: %v (downloaded %v)", name, err, downloaded)
		}
	}
	c, env := newComputer(t, "arm64")
	env.Exists = func(string) bool { return false }
	env.Release = ""
	if _, err := Prepare(context.Background(), env, c.root, c.user, ""); err == nil || !strings.Contains(err.Error(), "brew install e2fsprogs") {
		t.Errorf("a Mac without the disk tools is not told how to get them: %v", err)
	}
}

func TestAnImageThePersonNamesIsUsedWhereTheyKeepIt(t *testing.T) {
	c, env := newComputer(t, "arm64")
	image := filepath.Join(c.user, "mine.tar.gz")
	os.WriteFile(image, []byte("image"), 0o600)
	plan, err := Prepare(context.Background(), env, c.root, c.user, image)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if settings, _ := Installed(plan.Layout); settings.Image != image {
		t.Errorf("the image is %q", settings.Image)
	}
}

func TestAnInstallIsUsedByHaasButNeverOverAnOperatorsSetting(t *testing.T) {
	c, env := newComputer(t, "arm64")
	plan, _ := Prepare(context.Background(), env, c.root, c.user, "")
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	set := map[string]string{}
	Adopt(plan.Layout, func(name string) string { return map[string]string{"HAAS_OPENSHELL_IMAGE": "theirs"}[name] }, func(name, value string) { set[name] = value })
	if set["HAAS_OPENSHELL"] != plan.Program(CLI) || set["HAAS_OPENSHELL_GATEWAY"] != GatewayName {
		t.Errorf("the install is not used: %v", set)
	}
	if _, overridden := set["HAAS_OPENSHELL_IMAGE"]; overridden {
		t.Error("an operator's image was replaced")
	}
}

func TestAnArchiveHoldsOneProgramAndNothingElse(t *testing.T) {
	var out bytes.Buffer
	zipped := gzip.NewWriter(&out)
	writer := tar.NewWriter(zipped)
	for _, name := range []string{"a", "b"} {
		writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
		writer.Write([]byte("x"))
	}
	writer.Close()
	zipped.Close()
	archive := filepath.Join(t.TempDir(), "two.tar.gz")
	os.WriteFile(archive, out.Bytes(), 0o600)
	if err := unpackOne(archive, filepath.Join(filepath.Dir(archive), "program")); err == nil {
		t.Error("an archive with two files was unpacked")
	}
}

func TestTheGatewayIsConfiguredForVirtualMachinesAndRunsWithShortStateAndItsOwnTools(t *testing.T) {
	l := Layout{Root: "/h/openshell", User: "/Users/ada"}
	text := l.ConfigText()
	for _, want := range []string{`compute_driver = "vm"`, `guest_tls_ca = "/h/openshell/tls/ca.crt"`, "allow_driver_config = true", "version = 2"} {
		if !strings.Contains(text, want) {
			t.Errorf("the configuration lacks %q:\n%s", want, text)
		}
	}
	if state := l.State(); len(state) > 40 {
		t.Errorf("the state folder %q is too long for a socket path", state)
	}
	if !strings.HasPrefix(l.GatewayPath(), l.Bin()+":") {
		t.Errorf("the gateway does not look in haas's own folder first: %s", l.GatewayPath())
	}
}

func TestAMacWithNoDiskToolsGetsTheReleasesAndTheLicenceBesideThem(t *testing.T) {
	c, env := newComputer(t, "arm64")
	var out bytes.Buffer
	zipped := gzip.NewWriter(&out)
	writer := tar.NewWriter(zipped)
	for _, name := range []string{"mke2fs", "debugfs", "e2fsck", "COPYING", "SOURCE.txt"} {
		writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(name)), Typeflag: tar.TypeReg})
		writer.Write([]byte(name))
	}
	writer.Close()
	zipped.Close()
	sum := sha256.Sum256(out.Bytes())
	c.assets[ToolsAsset] = out.Bytes()
	c.assets[ToolsAsset+".sha256"] = []byte(hex.EncodeToString(sum[:]) + "  " + strconv.Itoa(out.Len()) + "  " + ToolsAsset + "\n")
	here := env.Exists
	env.Exists = func(path string) bool {
		return !strings.HasSuffix(path, "mke2fs") && !strings.HasSuffix(path, "debugfs") && !strings.HasSuffix(path, "e2fsck") && here(path)
	}
	plan, err := Prepare(context.Background(), env, c.root, c.user, "")
	if err != nil || !plan.FetchTools || !strings.Contains(plan.Words(), "disk tools") {
		t.Fatalf("the tools are not to be fetched: %v %v", err, plan.FetchTools)
	}
	if err := plan.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"mke2fs", "debugfs", "e2fsck"} {
		if info, err := os.Stat(filepath.Join(plan.Bin(), tool)); err != nil || info.Mode().Perm()&0o100 == 0 {
			t.Errorf("%s is not an executable program in the gateway's folder: %v", tool, err)
		}
	}
	if _, err := os.Stat(filepath.Join(plan.Root, "licenses", "COPYING")); err != nil {
		t.Errorf("the licence is not kept beside the tools: %v", err)
	}
}
