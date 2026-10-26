package cloud

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTheDemoFormAsksForACellWhenTheVisitorBeginsAndNotWhenDemosArePaused(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		var asked atomic.Int32
		f := newFront(t, demoSet, func(c *Config) { c.Prewarm = func() { asked.Add(1) } })
		if _, page := f.browser().get("/demo"); !strings.Contains(page, `data-warm="/demo/warm"`) || !strings.Contains(page, "/static/demo-warm.js") {
			t.Fatalf("the form does not ask for a cell:\n%s", page)
		}
		if status, _, _ := f.browser().post("/demo/warm", nil); status != http.StatusNoContent || asked.Load() != 1 {
			t.Errorf("%d, asked %d times", status, asked.Load())
		}
		if err := f.store.SetDemosPaused(true); err != nil {
			t.Fatal(err)
		}
		if status, _, _ := f.browser().post("/demo/warm", nil); status != http.StatusNoContent || asked.Load() != 1 {
			t.Errorf("paused demos asked for a cell: %d, asked %d times", status, asked.Load())
		}
	})
}

func TestOnePlaceCannotKeepAskingForCells(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		var asked atomic.Int32
		f := newFront(t, demoSet, func(c *Config) { c.PerAddress = 2; c.Prewarm = func() { asked.Add(1) } })
		for range 5 {
			f.browser().post("/demo/warm", nil)
		}
		if asked.Load() != 2 {
			t.Errorf("one address asked for %d cells", asked.Load())
		}
	})
}

func TestAHostThatOffersNoDemoStartsNoCell(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, func(c *Config) { c.Prewarm = func() { t.Error("a cell was asked for") } })
		if status, _, _ := f.browser().post("/demo/warm", nil); status != http.StatusNotFound {
			t.Errorf("%d", status)
		}
	})
}
