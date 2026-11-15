package roomlive

import "testing"

func TestTheFrameCacheNeverServesAFrameOfAnEarlierEpoch(t *testing.T) {
	c := newFrameCache(1 << 10)
	c.put("room", "epoch-1", 2, []byte("old"))
	if _, found := c.get("room", "epoch-2", 2); found {
		t.Fatal("the frame of the log before was served for the log that started again")
	}
	c.put("room", "epoch-2", 2, []byte("new"))
	for epoch, want := range map[string]string{"epoch-1": "old", "epoch-2": "new"} {
		if got, found := c.get("room", epoch, 2); !found || string(got) != want {
			t.Errorf("%s: %q %v, want %q", epoch, got, found, want)
		}
	}
	if _, found := c.get("other", "epoch-2", 2); found {
		t.Error("a frame of one room was served for another")
	}
}

func TestTheFrameCacheStaysWithinItsBytesAndDropsTheLeastRecentlyUsed(t *testing.T) {
	c := newFrameCache(10)
	c.put("r", "e", 1, []byte("12345"))
	c.put("r", "e", 2, []byte("12345"))
	c.get("r", "e", 1)
	c.put("r", "e", 3, []byte("12345"))
	if _, found := c.get("r", "e", 2); found {
		t.Error("the least recently used frame was kept")
	}
	if _, found := c.get("r", "e", 1); !found {
		t.Error("a frame used lately was dropped")
	}
	c.put("r", "e", 4, make([]byte, 11))
	if c.bytes > 10 {
		t.Errorf("the cache holds %d bytes of 10", c.bytes)
	}
}
