package mirror

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/helper"
)

// cloud is an agent's folder in memory, and says no the way the server does.
type cloud struct {
	mu       sync.Mutex
	files    map[string]string
	at       map[string]int64
	clock    int64
	cut      bool
	fail     map[string]error
	hashed   int
	onGet    func(path string)
	onUpload func(path string)
}

func newCloud() *cloud {
	return &cloud{files: map[string]string{}, at: map[string]int64{}, fail: map[string]error{}, clock: 1}
}

func hashOf(text string) string { h := sha256.Sum256([]byte(text)); return hex.EncodeToString(h[:]) }

// agent writes a file as the agent would, behind the sync's back.
func (c *cloud) agent(name, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clock += 1_000_000
	c.files[name], c.at[name] = text, c.clock
}

func (c *cloud) agentDeletes(name string) { c.mu.Lock(); delete(c.files, name); c.mu.Unlock() }

func (c *cloud) Files(context.Context) ([]helper.RemoteFile, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.fail["files"]; err != nil {
		return nil, false, err
	}
	var out []helper.RemoteFile
	for name, text := range c.files {
		out = append(out, helper.RemoteFile{Path: name, Size: int64(len(text)), ModifiedNs: c.at[name]})
	}
	return out, c.cut, nil
}

func (c *cloud) Hashes(_ context.Context, paths []string) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]string{}
	for _, name := range paths {
		if text, ok := c.files[name]; ok {
			out[name] = hashOf(text)
			c.hashed++
		}
	}
	return out, nil
}

func (c *cloud) Download(_ context.Context, name string, w io.Writer) error {
	c.mu.Lock()
	if err := c.fail["download "+name]; err != nil {
		c.mu.Unlock()
		return err
	}
	text, ok := c.files[name]
	hook := c.onGet
	c.mu.Unlock()
	if hook != nil {
		hook(name)
	}
	if !ok {
		return errors.New("not found")
	}
	_, err := io.WriteString(w, text)
	return err
}

func (c *cloud) Upload(_ context.Context, name string, body io.Reader, saw string, _ bool) (string, error) {
	c.mu.Lock()
	hook := c.onUpload
	c.mu.Unlock()
	if hook != nil {
		hook(name)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.fail["upload "+name]; err != nil {
		return "", err
	}
	current, there := c.files[name]
	switch {
	case !there && saw != absent, there && saw != hashOf(current):
		return "", &helper.Changed{Hash: hashOf(current)}
	}
	text, _ := io.ReadAll(body)
	c.clock += 1_000_000
	c.files[name], c.at[name] = string(text), c.clock
	return hashOf(string(text)), nil
}

func (c *cloud) Remove(_ context.Context, name, saw string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	current, there := c.files[name]
	switch {
	case !there && saw != absent, there && saw != hashOf(current):
		return &helper.Changed{Hash: hashOf(current)}
	}
	delete(c.files, name)
	return nil
}

type pair struct {
	t      *testing.T
	dir    string
	remote *cloud
	state  string
	engine *Engine
	clock  time.Time
}

func newPair(t *testing.T) *pair {
	t.Helper()
	root := t.TempDir()
	folder := filepath.Join(root, "work")
	os.MkdirAll(folder, 0o755)
	p := &pair{t: t, dir: folder, remote: newCloud(), state: filepath.Join(root, "state.json"), clock: time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)}
	p.engine = p.fresh()
	return p
}

// fresh is an engine over the same folders that remembers only what was written to the state file, as a restart does.
func (p *pair) fresh() *Engine {
	state, err := OpenState(p.state)
	if err != nil {
		p.t.Fatal(err)
	}
	ignore, err := ReadIgnore(p.dir)
	if err != nil {
		p.t.Fatal(err)
	}
	return &Engine{Root: p.dir, Remote: p.remote, State: state, Ignore: ignore, Now: func() time.Time { return p.clock }, Log: func(string, ...any) {}}
}

func (p *pair) write(name, text string) {
	p.t.Helper()
	full := filepath.Join(p.dir, filepath.FromSlash(name))
	os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		p.t.Fatal(err)
	}
	// A saved file is a newer file, even when the same second saved it.
	p.clock = p.clock.Add(time.Second)
	os.Chtimes(full, p.clock, p.clock)
}

func (p *pair) read(name string) (string, bool) {
	text, err := os.ReadFile(filepath.Join(p.dir, filepath.FromSlash(name)))
	return string(text), err == nil
}

func (p *pair) run() *Report {
	p.t.Helper()
	p.engine = p.fresh()
	report, err := p.engine.Once(context.Background())
	if err != nil {
		p.t.Fatal(err)
	}
	p.clock = p.clock.Add(time.Minute)
	return report
}

func (p *pair) localNames() []string {
	var names []string
	filepath.WalkDir(p.dir, func(path string, entry os.DirEntry, err error) error {
		if err == nil && entry.Type().IsRegular() {
			relative, _ := filepath.Rel(p.dir, path)
			names = append(names, filepath.ToSlash(relative))
		}
		return nil
	})
	sort.Strings(names)
	return names
}

func (p *pair) cloudNames() []string {
	var names []string
	for name := range p.remote.files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestAFirstRunBringsTheCloudFolderDownAndASecondDoesNothing(t *testing.T) {
	p := newPair(t)
	p.remote.agent("README.md", "# the project")
	p.remote.agent("src/main.go", "package main")
	first := p.run()
	if first.Done[Pull] != 2 {
		t.Fatalf("%+v", first.Done)
	}
	if text, _ := p.read("src/main.go"); text != "package main" {
		t.Errorf("%q", text)
	}
	second := p.run()
	if second.Done[Pull]+second.Done[Push]+second.Done[Conflict] != 0 || len(second.Errors) != 0 {
		t.Errorf("a run with nothing changed did %+v %v", second.Done, second.Errors)
	}
	p.remote.mu.Lock()
	before := p.remote.hashed
	p.remote.mu.Unlock()
	p.run()
	if p.remote.hashed != before {
		t.Errorf("files that did not change were hashed again: %d times", p.remote.hashed-before)
	}
}

func TestChangesOnEitherSideGoToTheOtherAndDeletionsToo(t *testing.T) {
	p := newPair(t)
	p.remote.agent("a.txt", "one")
	p.remote.agent("b.txt", "one")
	p.write("c.txt", "mine")
	p.run()
	p.write("a.txt", "edited here")
	p.remote.agent("b.txt", "edited there")
	p.write("d.txt", "new here")
	p.remote.agent("e.txt", "new there")
	os.Remove(filepath.Join(p.dir, "c.txt"))
	p.run()
	for name, want := range map[string]string{"a.txt": "edited here", "b.txt": "edited there", "d.txt": "new here", "e.txt": "new there"} {
		if got, _ := p.read(name); got != want {
			t.Errorf("local %s is %q, want %q", name, got, want)
		}
		if p.remote.files[name] != want {
			t.Errorf("cloud %s is %q, want %q", name, p.remote.files[name], want)
		}
	}
	if _, there := p.remote.files["c.txt"]; there {
		t.Error("a file deleted here is still in the cloud")
	}
	p.remote.agentDeletes("d.txt")
	p.run()
	if _, there := p.read("d.txt"); there {
		t.Error("a file deleted in the cloud is still here")
	}
}

func TestWhenBothSidesEditTheSameFileBothVersionsSurviveAndTheSidesAgree(t *testing.T) {
	p := newPair(t)
	p.remote.agent("report.md", "draft")
	p.run()
	p.write("report.md", "the person's rewrite")
	p.remote.agent("report.md", "the agent's rewrite")
	named := "report.conflict-" + p.clock.UTC().Format("20060102T150405Z") + ".md"
	report := p.run()
	if len(report.Conflicts) != 1 || report.Conflicts[0] != named {
		t.Fatalf("%+v", report.Conflicts)
	}
	if got, _ := p.read("report.md"); got != "the person's rewrite" {
		t.Errorf("the local file did not keep its name and its words: %q", got)
	}
	if got, _ := p.read(report.Conflicts[0]); got != "the agent's rewrite" {
		t.Errorf("the cloud's version was not saved beside it: %q", got)
	}
	if p.remote.files["report.md"] != "the person's rewrite" || p.remote.files[report.Conflicts[0]] != "the agent's rewrite" {
		t.Errorf("the cloud does not hold both: %v", p.cloudNames())
	}
	again := p.run()
	if again.Done[Conflict]+again.Done[Push]+again.Done[Pull] != 0 || len(again.Errors) != 0 {
		t.Errorf("after a conflict the sides still differ: %+v %v", again.Done, again.Errors)
	}
	if strings.Join(p.localNames(), ",") != strings.Join(p.cloudNames(), ",") {
		t.Errorf("%v vs %v", p.localNames(), p.cloudNames())
	}
}

func TestAChangeBeatsADeletionWhicheverSideChangedIt(t *testing.T) {
	p := newPair(t)
	p.remote.agent("keep-a.txt", "1")
	p.remote.agent("keep-b.txt", "1")
	p.run()
	os.Remove(filepath.Join(p.dir, "keep-a.txt"))
	p.remote.agent("keep-a.txt", "the agent edited what the person deleted")
	p.write("keep-b.txt", "the person edited what the agent deleted")
	p.remote.agentDeletes("keep-b.txt")
	p.run()
	if got, _ := p.read("keep-a.txt"); got != "the agent edited what the person deleted" {
		t.Errorf("keep-a: %q", got)
	}
	if p.remote.files["keep-b.txt"] != "the person's edit is kept" && p.remote.files["keep-b.txt"] != "the person edited what the agent deleted" {
		t.Errorf("keep-b in the cloud: %q", p.remote.files["keep-b.txt"])
	}
}

func TestAFileTheLocalPersonSavedWhileARunWasGoingIsNeverOverwritten(t *testing.T) {
	p := newPair(t)
	p.remote.agent("notes.md", "v1")
	p.run()
	p.remote.agent("notes.md", "the agent's v2")
	// The person saves the file after the run looked and before it writes: the pull must not take it.
	p.remote.onGet = func(string) { p.write("notes.md", "the person's v2, saved during the run"); p.remote.onGet = nil }
	first := p.run()
	if got, _ := p.read("notes.md"); got != "the person's v2, saved during the run" {
		t.Fatalf("the person's file was overwritten: %q", got)
	}
	if len(first.Errors) == 0 {
		t.Error("the run did not say it left the file alone")
	}
	p.run()
	names := p.localNames()
	var copies int
	for _, name := range names {
		if strings.Contains(name, ".conflict-") {
			copies++
		}
	}
	if got, _ := p.read("notes.md"); got != "the person's v2, saved during the run" || copies != 1 {
		t.Errorf("the next run did not keep both: %q and %v", got, names)
	}
}

func TestACloudFileThatChangedBeforeAnUploadIsNotOverwrittenAndIsHandledNextTime(t *testing.T) {
	p := newPair(t)
	p.remote.agent("a.txt", "v1")
	p.run()
	p.write("a.txt", "the person's v2")
	p.remote.onUpload = func(string) { p.remote.agent("a.txt", "the agent's v2, written just now"); p.remote.onUpload = nil }
	// the agent's write is made to land in the middle of the run's own upload, which the server must refuse
	first := p.run()
	if p.remote.files["a.txt"] != "the agent's v2, written just now" {
		t.Fatalf("the cloud's newer file was overwritten: %q", p.remote.files["a.txt"])
	}
	if len(first.Errors) == 0 {
		t.Error("the refused upload was not reported")
	}
	p.run()
	if got, _ := p.read("a.txt"); got != "the person's v2" {
		t.Errorf("the local file: %q", got)
	}
	found := false
	for name, text := range p.remote.files {
		if strings.Contains(name, ".conflict-") && text == "the agent's v2, written just now" {
			found = true
		}
	}
	if !found {
		t.Errorf("the agent's version was lost: %v", p.cloudNames())
	}
}

func TestAConflictThatCannotBeSentLeavesBothSidesAsTheyWereAndNoPileOfCopies(t *testing.T) {
	p := newPair(t)
	p.remote.agent("a.txt", "v1")
	p.run()
	p.write("a.txt", "mine")
	p.remote.agent("a.txt", "theirs")
	p.remote.fail["upload a.txt"] = errors.New("the network went")
	for i := 0; i < 3; i++ {
		report := p.run()
		if len(report.Errors) == 0 {
			t.Fatal("the failure was not reported")
		}
	}
	if got, _ := p.read("a.txt"); got != "mine" || p.remote.files["a.txt"] != "theirs" {
		t.Errorf("a failed conflict changed a side: %q / %q", got, p.remote.files["a.txt"])
	}
	for _, name := range p.localNames() {
		if strings.Contains(name, ".conflict-") {
			t.Errorf("a failed conflict left a copy behind: %s", name)
		}
	}
	delete(p.remote.fail, "upload a.txt")
	p.run()
	if len(p.localNames()) != 2 || strings.Join(p.localNames(), ",") != strings.Join(p.cloudNames(), ",") {
		t.Errorf("after the network came back: %v vs %v", p.localNames(), p.cloudNames())
	}
}

func TestLinksAreNotFollowedAndLargeFilesAreLeftAloneAndNotTakenForDeleted(t *testing.T) {
	p := newPair(t)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret"), []byte("not for the cloud"), 0o644)
	link(t, outside, filepath.Join(p.dir, "link"))
	link(t, filepath.Join(outside, "secret"), filepath.Join(p.dir, "linkfile"))
	p.write("ok.txt", "fine")
	p.engine = p.fresh()
	p.engine.MaxFileBytes = 10
	p.write("big.bin", strings.Repeat("x", 100))
	p.remote.agent("cloudbig.bin", strings.Repeat("y", 100))
	report, err := p.engine.Once(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.cloudNames(), ",") != "cloudbig.bin,ok.txt" {
		t.Errorf("the cloud holds %v", p.cloudNames())
	}
	if len(report.Skipped.Links) != 2 || len(report.Skipped.Large) != 2 {
		t.Errorf("%+v", report.Skipped)
	}
	// The large cloud file is not here, and its absence is not a deletion of anything. Run again, and again after it shrinks.
	p.engine = p.fresh()
	p.engine.MaxFileBytes = 10
	if _, err := p.engine.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, there := p.remote.files["cloudbig.bin"]; !there {
		t.Error("a large file in the cloud was deleted")
	}
	if _, there := p.read("big.bin"); !there {
		t.Error("a large local file was deleted")
	}
}

func TestAPathFromTheCloudThatWouldLeaveTheFolderIsRefusedAndNothingIsWritten(t *testing.T) {
	p := newPair(t)
	outside := filepath.Join(filepath.Dir(p.dir), "escaped.txt")
	link(t, t.TempDir(), filepath.Join(p.dir, "dirlink"))
	p.remote.agent("../escaped.txt", "bad")
	p.remote.agent("dirlink/inside.txt", "bad")
	p.remote.agent("/abs.txt", "bad")
	p.remote.agent("fine.txt", "good")
	report := p.run()
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("a file was written outside the folder")
	}
	if got, _ := p.read("fine.txt"); got != "good" {
		t.Errorf("the good file was not pulled: %q", got)
	}
	if len(report.Errors) < 3 {
		t.Errorf("the refused paths were not reported: %v", report.Errors)
	}
	entries, _ := os.ReadDir(p.dir)
	for _, entry := range entries {
		if entry.Name() == "abs.txt" {
			t.Error("an absolute path was made relative and written")
		}
	}
}

func TestWhatLooksLikeAMistakeIsRefusedBeforeAnythingIsDone(t *testing.T) {
	p := newPair(t)
	for i := 0; i < 30; i++ {
		p.remote.agent("f"+string(rune('a'+i%26))+string(rune('a'+i/26))+".txt", "x")
	}
	p.run()
	// The disk did not mount: the folder is empty.
	for _, name := range p.localNames() {
		os.Remove(filepath.Join(p.dir, name))
	}
	p.engine = p.fresh()
	if _, err := p.engine.Once(context.Background()); !errors.Is(err, ErrGuard) {
		t.Fatalf("an emptied folder was carried to the cloud: %v", err)
	}
	if len(p.remote.files) != 30 {
		t.Errorf("the cloud lost files: %d left", len(p.remote.files))
	}
	p.engine = p.fresh()
	p.engine.Force = true
	if _, err := p.engine.Once(context.Background()); err != nil || len(p.remote.files) != 0 {
		t.Errorf("a forced run did not go through: %v, %d left", err, len(p.remote.files))
	}
}

func TestACutListingAndTheHomeFolderAndTheRootOfADiskAreRefused(t *testing.T) {
	p := newPair(t)
	p.remote.agent("a.txt", "1")
	p.remote.cut = true
	if _, err := p.fresh().Once(context.Background()); err == nil || !strings.Contains(err.Error(), "more files than can be listed") {
		t.Errorf("a cut listing, in which files that are not listed look deleted, was used: %v", err)
	}
	p.remote.cut = false
	home, _ := os.UserHomeDir()
	for _, root := range []string{home, "/", filepath.Join(p.dir, "nope")} {
		engine := p.fresh()
		engine.Root = root
		if _, err := engine.Once(context.Background()); err == nil {
			t.Errorf("%q was kept in step", root)
		}
	}
}

func TestIgnoredFilesNeverCrossAndAreNeverDeleted(t *testing.T) {
	p := newPair(t)
	p.write(".gustoignore", "*.tmp\nscratch/\n")
	p.write("node_modules/pkg/index.js", "x")
	p.write("work.tmp", "x")
	p.write("scratch/a", "x")
	p.write("kept.txt", "kept")
	p.remote.agent("node_modules/dep/index.js", "x")
	p.remote.agent("cloud.tmp", "x")
	p.run()
	if strings.Join(p.cloudNames(), ",") != "cloud.tmp,kept.txt,node_modules/dep/index.js" {
		t.Errorf("the cloud holds %v", p.cloudNames())
	}
	if _, there := p.read("cloud.tmp"); there {
		t.Error("an ignored cloud file came down")
	}
	p.run()
	if _, there := p.read("node_modules/pkg/index.js"); !there {
		t.Error("an ignored local file was deleted")
	}
}

func TestTheRecordSurvivesARestartAndAnUnreadableOneIsNotTakenForNothing(t *testing.T) {
	p := newPair(t)
	p.remote.agent("a.txt", "1")
	p.run()
	p.remote.agentDeletes("a.txt")
	p.run()
	if _, there := p.read("a.txt"); there {
		t.Error("a deletion in the cloud was not carried out after the engine was started again")
	}
	os.WriteFile(p.state, []byte("{not json"), 0o600)
	if _, err := OpenState(p.state); err == nil || !strings.Contains(err.Error(), "move it away") {
		t.Errorf("an unreadable record was taken for an empty one: %v", err)
	}
	var out bytes.Buffer
	_ = out
}

func TestKeepRunsAgainAndAgainAndStopsWhenAskedAndWaitsLongerWhenNothingChanges(t *testing.T) {
	p := newPair(t)
	p.remote.agent("a.txt", "1")
	engine := p.fresh()
	ctx, stop := context.WithCancel(context.Background())
	var times []time.Time
	var failed int
	done := make(chan struct{})
	go func() {
		engine.Keep(ctx, 10*time.Millisecond, 80*time.Millisecond, nil, func(report *Report, err error) {
			times = append(times, time.Now())
			if err != nil {
				failed++
			}
			if len(times) == 8 {
				stop()
			}
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Keep did not stop when asked")
	}
	if failed != 0 || len(times) < 8 {
		t.Fatalf("%d runs, %d failed", len(times), failed)
	}
	first, last := times[2].Sub(times[1]), times[7].Sub(times[6])
	if last <= first {
		t.Errorf("an idle folder was looked at as often as a busy one: %v then %v", first, last)
	}
	if text, _ := p.read("a.txt"); text != "1" {
		t.Errorf("the first run did not pull: %q", text)
	}
}

// link makes a symbolic link, or skips the test where this system will not let a person make one.
func link(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		t.Skip("this system does not let a link be made here: " + err.Error())
	}
}
