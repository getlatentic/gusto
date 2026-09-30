package web

import "testing"

func TestAnAgentsFolderIsLeftOffTheFilesItNamesWhereverTheFolderIs(t *testing.T) {
	for in, want := range map[string]string{
		"/job/workspace/adder.sh": "adder.sh",
		"/job/workspace":          "workspace",
		"/sandbox/jobs/instance-20260930-2330-engineering/workspace/2048-app/index.html": "2048-app/index.html",
		"/sandbox/jobs/instance-20260930-2330-engineering/workspace":                     "workspace",
		"cd /sandbox/jobs/i-1/workspace && ls":                                           "cd workspace && ls",
		"cp /sandbox/jobs/i-1/workspace/a /sandbox/jobs/i-1/workspace/b":                 "cp a b",
		"mkdir -p 2048-app":                "mkdir -p 2048-app",
		"/etc/passwd":                      "/etc/passwd",
		"/sandbox/jobs/i-1/notworkspace/a": "/sandbox/jobs/i-1/notworkspace/a",
		"/home/user/workspace/a":           "/home/user/workspace/a",
	} {
		if got := tidy(in); got != want {
			t.Errorf("%q: %q, wanted %q", in, got, want)
		}
	}
}
