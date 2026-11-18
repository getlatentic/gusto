package main

import (
	"testing"

	"github.com/getlatentic/gusto/internal/runner"
)

func TestInsideAppReadsTheAppsSocketPortFolderAndOneLineCommand(t *testing.T) {
	got, err := insideAppArgs([]string{"--socket", "/run/gusto/app/app.sock", "--port", "3000", "--dir", "/job/workspace", "--", "npm run start -- --port 3000"})
	want := runner.App{Socket: "/run/gusto/app/app.sock", Port: 3000, Folder: "/job/workspace", Command: "npm run start -- --port 3000"}
	if err != nil || got != want {
		t.Fatalf("got %+v, %v", got, err)
	}
	for _, wrong := range [][]string{
		{"--socket", "/s", "--port", "3000", "--dir", "/d"},
		{"--socket", "/s", "--port", "x", "--dir", "/d", "--", "c"},
		{"--port", "3000", "--dir", "/d", "--", "c"},
		{"--socket", "/s", "--port", "3000", "--dir", "/d", "--", "c", "extra"},
		{"--network", "open", "--socket", "/s", "--port", "3000", "--dir", "/d", "--", "c"},
	} {
		if _, err := insideAppArgs(wrong); err == nil {
			t.Errorf("%q was taken", wrong)
		}
	}
}
