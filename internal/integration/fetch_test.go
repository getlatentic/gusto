package integration

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type member struct {
	name, text string
	mode       int64
	link       string
}

func zipOf(t *testing.T, entries ...member) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, e := range entries {
		header := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		switch {
		case e.link != "":
			header.SetMode(os.ModeSymlink | 0o777)
		case e.mode != 0:
			header.SetMode(os.FileMode(e.mode))
		}
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		file.Write([]byte(e.text + e.link))
	}
	writer.Close()
	return out.Bytes()
}

func tarGzOf(t *testing.T, entries ...member) []byte {
	t.Helper()
	var out bytes.Buffer
	zipped := gzip.NewWriter(&out)
	writer := tar.NewWriter(zipped)
	for _, e := range entries {
		header := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.text)), Typeflag: tar.TypeReg}
		if e.mode != 0 {
			header.Mode = e.mode
		}
		if e.link != "" {
			header.Typeflag, header.Linkname, header.Size = tar.TypeSymlink, e.link, 0
		}
		if strings.HasSuffix(e.name, "/") {
			header.Typeflag, header.Size = tar.TypeDir, 0
		}
		writer.WriteHeader(header)
		writer.Write([]byte(e.text))
	}
	writer.Close()
	zipped.Close()
	return out.Bytes()
}

var plugin = []member{
	{name: "plugin.json", text: `{` + pluginSchema + `, "name": "reports"}`},
	{name: "skills/summarize/SKILL.md", text: skillText("summarize", "d")},
	{name: "skills/summarize/scripts/run.sh", text: "#!/bin/sh\n", mode: 0o755},
}

func TestAnUploadedPluginIsUnpackedAndReadFromZipAndTarGz(t *testing.T) {
	for name, data := range map[string][]byte{"zip": zipOf(t, plugin...), "tar.gz": tarGzOf(t, plugin...)} {
		dir, err := Unpack(data, t.TempDir())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		read, err := Read(dir)
		if err != nil || len(read.Skills) != 1 {
			t.Fatalf("%s: %+v %v", name, read, err)
		}
		if info, err := os.Stat(filepath.Join(dir, "skills/summarize/scripts/run.sh")); err != nil || info.Mode()&0o111 == 0 {
			t.Errorf("%s: a script lost its runnable bit", name)
		}
	}
}

func TestAnArchiveMadeFromARepositoryHasItsOneTopFolderTakenOff(t *testing.T) {
	var inside []member
	for _, e := range plugin {
		e.name = "reports-main/" + e.name
		inside = append(inside, e)
	}
	dir, err := Unpack(zipOf(t, inside...), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestAnArchiveThatWouldWriteOutsideItselfOrMakeALinkIsRefused(t *testing.T) {
	bad := map[string][]member{
		"a name that climbs":       {{name: "../evil", text: "x"}},
		"a name that climbs later": {{name: "a/../../evil", text: "x"}},
		"an absolute name":         {{name: "/etc/evil", text: "x"}},
		"a drive":                  {{name: "C:/evil", text: "x"}},
		"a back slash climb":       {{name: `..\evil`, text: "x"}},
		"a link":                   {{name: "plugin.json", text: "{}"}, {name: "skills", link: "/etc"}},
		"a name written twice":     {{name: "a", text: "1"}, {name: "a", text: "2"}},
		"a very deep name":         {{name: strings.Repeat("d/", MaxDepth+2) + "f", text: "x"}},
	}
	for name, entries := range bad {
		for kind, data := range map[string][]byte{"zip": zipOf(t, entries...), "tar.gz": tarGzOf(t, entries...)} {
			parent := t.TempDir()
			if _, err := Unpack(data, parent); err == nil || !strings.Contains(err.Error(), "not an Agent Plugin") {
				t.Errorf("%s (%s): err = %v", name, kind, err)
			}
			if left, _ := os.ReadDir(parent); len(left) != 0 {
				t.Errorf("%s (%s): a refused archive left %d things behind", name, kind, len(left))
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(parent), "evil")); err == nil {
				t.Errorf("%s (%s): a file was written outside", name, kind)
			}
		}
	}
	for _, data := range [][]byte{[]byte("not an archive"), nil, []byte("PK\x03\x04junk"), []byte{0x1f, 0x8b, 0}} {
		if _, err := Unpack(data, t.TempDir()); err == nil {
			t.Errorf("%q was unpacked", data)
		}
	}
}

func TestAnArchiveThatUnpacksToTooMuchStopsAtTheLimit(t *testing.T) {
	small := zipOf(t, member{name: "big", text: strings.Repeat("a", 4<<20)})
	if len(small) > 100<<10 {
		t.Fatalf("the test archive is %d bytes, and should be small", len(small))
	}
	parent := t.TempDir()
	if _, err := unpack(small, parent, 10, 1<<20); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Errorf("a small archive that unpacks to 4 MiB was taken at a limit of 1: %v", err)
	}
	if left, _ := os.ReadDir(parent); len(left) != 0 {
		t.Error("the refused archive left what it had written")
	}
	var files []member
	for i := range 11 {
		files = append(files, member{name: "d/f" + itoa(i), text: "x"})
	}
	if _, err := unpack(tarGzOf(t, files...), t.TempDir(), 10, MaxBytes); err == nil {
		t.Error("an archive of more files than the limit was unpacked")
	}
	if _, err := Unpack(make([]byte, MaxUpload+1), t.TempDir()); err == nil {
		t.Error("an upload over the limit was taken")
	}
}

func repository(t *testing.T, files map[string]string) (address, commit string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := folder(t, files)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "x"}} {
		command := exec.Command("git", args...)
		command.Dir = dir
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = dir
	out, _ := command.Output()
	return "file://" + dir, strings.TrimSpace(string(out))
}

func TestAPluginIsFetchedFromAGitAddressAndPinnedToItsCommit(t *testing.T) {
	address, commit := repository(t, map[string]string{
		"plugin.json": manifest(""), "skills/a/SKILL.md": skillText("a", "d"),
		"packages/inner/plugin.json": `{` + pluginSchema + `, "name": "inner"}`,
		"escape":                     "-> /etc/hostname",
	})
	clone := Clone{Schemes: []string{"file"}}
	dir, source, err := clone.Fetch(context.Background(), address, "main", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if source.Commit != commit || source.Kind != "git" || source.Ref != "main" {
		t.Errorf("source = %+v, want the commit %s", source, commit)
	}
	read, err := Read(dir)
	if err != nil || len(read.Skills) != 1 {
		t.Fatalf("%+v %v", read, err)
	}
	if info, err := os.Lstat(filepath.Join(dir, "escape")); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Errorf("a link in the repository was made a link: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Log("the clone's .git is not kept in the store, which skips it")
	}
	inner, _, err := clone.Fetch(context.Background(), address, commit, "packages/inner", t.TempDir())
	if err != nil {
		t.Fatalf("a commit and a folder: %v", err)
	}
	if read, err := Read(inner); err != nil || read.Name != "inner" {
		t.Errorf("%+v %v", read, err)
	}
}

func TestAnAddressOrARefThatIsNotAPluginSourceIsRefusedBeforeAnythingIsRun(t *testing.T) {
	https := Clone{Resolve: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("93.184.216.34")}, nil }}
	parent := t.TempDir()
	for name, address := range map[string]string{
		"a local path":    "/tmp/repo",
		"a file address":  "file:///tmp/repo",
		"ssh":             "ssh://git@example.com/repo.git",
		"a password":      "https://u:p@example.com/repo.git",
		"an option":       "--upload-pack=touch /tmp/x",
		"a relative path": "../repo",
		"plain http":      "http://example.com/repo.git",
		"an ext helper":   "ext::sh -c touch% /tmp/x",
	} {
		if _, _, err := https.Fetch(context.Background(), address, "", "", parent); err == nil {
			t.Errorf("%s: the address was taken", name)
		}
	}
	if _, _, err := https.Fetch(context.Background(), "https://u:p@example.com/repo.git", "", "", parent); err == nil || !strings.Contains(err.Error(), "user name") {
		t.Errorf("an address with a password: err = %v", err)
	}
	for _, ref := range []string{"--upload-pack=x", "a b", "a:b", "-x", "a\nb", "a~1", "a^"} {
		if _, _, err := https.Fetch(context.Background(), "https://example.com/repo.git", ref, "", parent); err == nil || !strings.Contains(err.Error(), "not a branch") {
			t.Errorf("ref %q: err = %v", ref, err)
		}
	}
	if left, _ := os.ReadDir(parent); len(left) != 0 {
		t.Errorf("a refused fetch left %d things behind", len(left))
	}
}

func TestAnAddressOnAPrivateNetworkIsRefusedUnlessAnAdministratorAllowsIt(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.0.5", "172.16.0.1", "169.254.169.254", "::1", "fd00::1", "0.0.0.0"} {
		clone := Clone{Resolve: func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP(ip)}, nil }}
		if _, _, err := clone.Fetch(context.Background(), "https://git.example.com/repo.git", "", "", t.TempDir()); err == nil || !strings.Contains(err.Error(), "private network") {
			t.Errorf("%s: err = %v", ip, err)
		}
	}
	mixed := Clone{Resolve: func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("10.0.0.1")}, nil
	}}
	if _, _, err := mixed.Fetch(context.Background(), "https://git.example.com/repo.git", "", "", t.TempDir()); err == nil || !strings.Contains(err.Error(), "private network") {
		t.Errorf("a name with one private address was taken: %v", err)
	}
}

func TestASubfolderThatLeavesTheRepositoryIsRefused(t *testing.T) {
	address, _ := repository(t, map[string]string{"plugin.json": manifest("")})
	clone := Clone{Schemes: []string{"file"}}
	for _, sub := range []string{"..", "../x", "/etc", "nope"} {
		if _, _, err := clone.Fetch(context.Background(), address, "", sub, t.TempDir()); err == nil {
			t.Errorf("%q: the folder was taken", sub)
		}
	}
}
