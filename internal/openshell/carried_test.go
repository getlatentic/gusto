package openshell

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/catalog"
)

type counting struct {
	asked  int
	answer catalog.Carried
}

func (c *counting) Carried([]string) catalog.Carried { c.asked++; return c.answer }

func imageFile(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runner.tar.gz")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnImageIsAskedOnceAndAServerThatStartsAfterReadsTheAnswer(t *testing.T) {
	path := imageFile(t, "image")
	inner := &counting{answer: catalog.Carried{OnPath: map[string]bool{"opencode": true, "codex": false}}}
	first := Remembered(inner, path).Carried([]string{"opencode", "codex"})
	// Another server, another process: nothing but the file is shared.
	second := Remembered(inner, path).Carried([]string{"codex", "opencode"})
	if inner.asked != 1 {
		t.Errorf("the image was asked %d times", inner.asked)
	}
	if !first.OnPath["opencode"] || !second.OnPath["opencode"] || second.OnPath["codex"] {
		t.Errorf("%v then %v", first.OnPath, second.OnPath)
	}
}

func TestAnImageMadeAgainOrAskedAboutMoreIsAskedAgain(t *testing.T) {
	path := imageFile(t, "image")
	inner := &counting{answer: catalog.Carried{OnPath: map[string]bool{"opencode": true}}}
	Remembered(inner, path).Carried([]string{"opencode"})
	Remembered(inner, path).Carried([]string{"opencode", "claude"})
	if inner.asked != 2 {
		t.Errorf("a program that was not asked about before is asked about: %d", inner.asked)
	}
	// The same size, made later.
	os.WriteFile(path, []byte("IMAGE"), 0o644)
	os.Chtimes(path, time.Now().Add(time.Hour), time.Now().Add(time.Hour))
	Remembered(inner, path).Carried([]string{"opencode", "claude"})
	if inner.asked != 3 {
		t.Errorf("an image that changed is asked again: %d", inner.asked)
	}
	// A different size.
	os.WriteFile(path, []byte("a longer image"), 0o644)
	Remembered(inner, path).Carried([]string{"opencode", "claude"})
	if inner.asked != 4 {
		t.Errorf("%d", inner.asked)
	}
}

func TestAnAnswerThatIsNotOneIsNotKept(t *testing.T) {
	path := imageFile(t, "image")
	inner := &counting{answer: catalog.Carried{Unanswered: errors.New("no gateway")}}
	Remembered(inner, path).Carried([]string{"opencode"})
	inner.answer = catalog.Carried{OnPath: map[string]bool{"opencode": true}}
	got := Remembered(inner, path).Carried([]string{"opencode"})
	if inner.asked != 2 || !got.OnPath["opencode"] {
		t.Errorf("a failure was remembered: asked %d, %v", inner.asked, got)
	}
	if _, err := os.Stat(path + ".carried.json"); err != nil {
		t.Error("a real answer is kept")
	}
}

func TestAnImageWithNoFileIsAskedEveryTime(t *testing.T) {
	inner := &counting{answer: catalog.Carried{OnPath: map[string]bool{}}}
	image := Remembered(inner, filepath.Join(t.TempDir(), "none"))
	image.Carried([]string{"a"})
	image.Carried([]string{"a"})
	if inner.asked != 2 {
		t.Errorf("%d", inner.asked)
	}
}

func TestACorruptAnswerIsAskedAgain(t *testing.T) {
	path := imageFile(t, "image")
	os.WriteFile(path+".carried.json", []byte("{not json"), 0o644)
	inner := &counting{answer: catalog.Carried{OnPath: map[string]bool{"a": true}}}
	if got := Remembered(inner, path).Carried([]string{"a"}); inner.asked != 1 || !got.OnPath["a"] {
		t.Errorf("%d %v", inner.asked, got)
	}
}
