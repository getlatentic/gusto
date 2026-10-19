package main

import "testing"

func TestDoctorChecksTheInstallOnlyWhenThereIsOneAndTheShellNamesNoServer(t *testing.T) {
	has := func(paths ...string) func(string) bool {
		return func(p string) bool {
			for _, want := range paths {
				if p == want {
					return true
				}
			}
			return false
		}
	}
	env := func(set map[string]string) func(string) string { return func(n string) string { return set[n] } }
	installed := has("/home/ada/.local/share/gusto/local.json")
	for _, c := range []struct {
		name   string
		exists func(string) bool
		env    map[string]string
		want   bool
	}{
		{"an install and a bare shell", installed, nil, true},
		{"no install", has(), nil, false},
		{"a folder of tasks is named", installed, map[string]string{"GUSTO_JOBS": "/srv/jobs"}, false},
		{"an address is named", installed, map[string]string{"GUSTO_ADDR": ":8040"}, false},
		{"a sandbox is named", installed, map[string]string{"GUSTO_SANDBOX": "container"}, false},
		{"a container command is named", installed, map[string]string{"GUSTO_CONTAINER": "podman"}, false},
	} {
		if got := checksLocalInstall("/home/ada/.local/share/gusto", env(c.env), c.exists); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
