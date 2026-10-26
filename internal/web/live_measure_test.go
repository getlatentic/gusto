package web

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/roomlog"
)

func TestTheLiveLayerIsReportedOnTheMetricsPage(t *testing.T) {
	l := newLiveRoom(t, &roomWork{}, [2]string{"Scout", "fixture"})
	ada := l.socket(alice)
	l.say(ada, "c-1", "hello", false)
	frames(t, ada, 1)

	var page bytes.Buffer
	l.site.server.liveMetrics(&exposition{w: &page})
	for _, want := range []string{"gusto_live_connections 1\n", "gusto_live_rooms_followed 1\n", "gusto_live_commands_total 1\n"} {
		if !strings.Contains(page.String(), want) {
			t.Errorf("the metrics lack %q:\n%s", want, page.String())
		}
	}
	if strings.Contains(page.String(), "gusto_live_events_drawn_total 0\n") {
		t.Errorf("an event was handed on but none was drawn:\n%s", page.String())
	}
}

// What drawing one turn for the room costs, for a conversation of each length, since a live update is drawn once for everybody and
// the old page drew every turn of the conversation for each viewer on each change.
//
//	ROOMLIVE_MEASURE=1 go test -run TestMeasureDrawingATurn -count=1 -v ./internal/web/
func TestMeasureDrawingATurn(t *testing.T) {
	if os.Getenv("ROOMLIVE_MEASURE") == "" {
		t.Skip("a measurement, not a test: set ROOMLIVE_MEASURE=1")
	}
	work := &roomWork{replies: map[string]string{"a1": strings.Repeat("A reply of some length, as an agent writes. ", 40)}}
	l := newLiveRoom(t, work, [2]string{"Scout", "fixture"})
	mine := l.site.as(t, alice)
	made := 0
	for _, length := range []int{10, 100, 500} {
		for ; made < length; made++ {
			mine.ask(t, "POST", l.thread+"/say", map[string][]string{"said": {fmt.Sprintf("question %d", made)}})
		}
		work.waitTurns(t, l.site, length)
		held, _ := l.site.server.instances.Get(l.id)
		thread := l.site.server.instances.Latest(l.id)
		last := thread.Turns[len(thread.Turns)-1]

		const rounds = 200
		began := time.Now()
		for range rounds {
			if _, err := l.site.server.drawItem(held, roomlog.Event{Kind: "turn", Thread: thread.ID, Ref: last}); err != nil {
				t.Fatal(err)
			}
		}
		drawn := time.Since(began) / rounds

		began = time.Now()
		for range 20 {
			if rec := l.site.as(t, alice).ask(t, "GET", l.thread+"/turns", nil); rec.Code != 200 {
				t.Fatal(rec.Code)
			}
		}
		page := time.Since(began) / 20
		t.Logf("a conversation of %3d turns: one live update drawn in %v, the whole conversation as the old page drew it on every change in %v", length, drawn.Round(time.Microsecond), page.Round(time.Microsecond))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = l.site.server.CloseLive(ctx)
}
