package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/roomwork"
)

func TestARoomTooBigToCopyRefusesTheTurnAndSaysWhy(t *testing.T) {
	t.Setenv("GUSTO_ROOM_COPY_MB", "1")
	rooms, held := aRoomWithTwoAgents(t, map[string]string{"big.bin": strings.Repeat("x", 2<<20)})
	dir, spec := turnOf(t, rooms, held, "a1")
	ran := false
	_, err := rooms.around(t.Context(), spec, dir, func() (int, error) { ran = true; return 0, nil })
	var big *roomwork.TooBigError
	if !errors.As(err, &big) || ran {
		t.Fatalf("the turn ran on a folder over the limit: %v %v", ran, err)
	}
	said, _ := os.ReadFile(dir.OutFile("transcript.log"))
	if !strings.Contains(string(said), "more than a turn can work on a copy of") {
		t.Errorf("the record does not say why: %q", said)
	}
}
