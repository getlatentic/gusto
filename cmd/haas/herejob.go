package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/runner"
)

// here runs a job in this process, which is what a person's own computer has until it has a sandbox to give a job. A turn in a
// container gets the instance's workspace and state mounted over its own; here the job's workspace is made a link to the
// instance's, and the harness is given a home of the job's own with the instance's state as its data, so a conversation works
// the same way in both places.
func here(way *WayOut, instances *instance.Store) jobs.StoppableWork {
	return func(ctx context.Context, dir job.Dir) (int, error) {
		spec, err := job.Read(dir)
		if err != nil {
			return 0, err
		}
		if spec.Session != "" {
			if err := pointAt(instances.Workspace(spec.Session), dir); err != nil {
				return 0, err
			}
		}
		home, data, err := jobHome(dir, instances, spec)
		if err != nil {
			return 0, err
		}
		env, err := way.hereEnv(dir, spec)
		if err != nil {
			return 0, err
		}
		env.Home, env.Data = home, data
		env.Base = homedEnv(env.Base, home, data, sharedCache())
		if os.Getenv("HAAS_SANDBOX") == "process" {
			if env.Wrap, err = behindSeatbelt(dir, spec, home, data, way.proxyPort()); err != nil {
				return 0, err
			}
		}
		defer way.Done(dir)
		return runner.InsideWith(ctx, dir, env)
	}
}

// jobHome is the home a harness run here is given, as a container's /root is: a folder of the job's own, holding each harness's
// configuration as the image does, and the instance's state as its data, which is what a container mounts at /root/.local/share.
func jobHome(dir job.Dir, instances *instance.Store, spec job.Spec) (home, data string, err error) {
	if home, err = filepath.Abs(filepath.Join(dir.Path(), "home")); err != nil {
		return "", "", err
	}
	data = filepath.Join(home, ".local", "share")
	if spec.Session != "" {
		if data, err = filepath.Abs(instances.State(spec.Session)); err != nil {
			return "", "", err
		}
	}
	for _, folder := range []string{home, data} {
		if err := os.MkdirAll(folder, 0o700); err != nil {
			return "", "", err
		}
	}
	return home, data, harness.PlaceConfig(home)
}

// hereEnv is what a harness run in the server's own process is started with: the proxy, on this machine's loopback, and the token
// that stands in for the job's credential, so its calls are capped and recorded as a box's are, and of the server's environment only
// what a program needs to run. Without a proxy the harness reaches the model it was configured with directly.
func (w *WayOut) hereEnv(dir job.Dir, spec job.Spec) (harness.Env, error) {
	url, token := w.direct, ""
	if w.proxy != nil {
		minted, err := w.tokenFor(dir, spec)
		if err != nil {
			return harness.Env{}, err
		}
		url, token = "http://127.0.0.1:"+w.proxyPort()+"/model/v1", minted
	}
	base := append(plainEnv(), "HAAS_MODEL_URL="+url)
	if token != "" {
		base = append(base, "HAAS_MODEL_TOKEN="+token)
	}
	return harness.Env{Base: base, ModelURL: url, ModelToken: token}, nil
}

// sharedCache is where a harness run here keeps what it downloads, such as the packages a harness installs into its own
// configuration: one folder for every job, so a turn does not download them again.
func sharedCache() string {
	if cache, err := os.UserCacheDir(); err == nil {
		return filepath.Join(cache, "haas")
	}
	return filepath.Join(os.TempDir(), "haas-cache")
}

// homedEnv is env with the harness's home and data put where a program looks for them, so a harness reads the configuration
// haas gave it and not the person's own, and its downloads in cache.
func homedEnv(env []string, home, data, cache string) []string {
	homes := []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA"}
	env = slices.DeleteFunc(slices.Clone(env), func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		return slices.ContainsFunc(homes, func(home string) bool { return strings.EqualFold(home, name) })
	})
	env = append(env, "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "XDG_DATA_HOME="+data,
		"XDG_STATE_HOME="+filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME="+cache)
	if runtime.GOOS == "windows" {
		env = append(env, "USERPROFILE="+home, "APPDATA="+filepath.Join(home, "AppData", "Roaming"),
			"LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"))
	}
	return env
}

// plainEnv is the server's environment without anything that is the server's own: where programs are, who the person is, their
// language and terminal, and on Windows what a program needs to start at all. A password, a key or a token of the server is in
// none of it.
func plainEnv() []string {
	kept := []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "LANG", "TERM", "TMPDIR", "TZ"}
	if runtime.GOOS == "windows" {
		kept = append(kept, "SystemRoot", "SystemDrive", "windir", "ComSpec", "PATHEXT", "TEMP", "TMP", "USERNAME", "USERPROFILE",
			"APPDATA", "LOCALAPPDATA", "ProgramData", "ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "CommonProgramFiles",
			"CommonProgramFiles(x86)", "CommonProgramW6432", "PROCESSOR_ARCHITECTURE", "NUMBER_OF_PROCESSORS", "OS")
	}
	var env []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if slices.ContainsFunc(kept, func(k string) bool { return strings.EqualFold(k, name) }) ||
			strings.HasPrefix(name, "LC_") || strings.HasPrefix(name, "XDG_") {
			env = append(env, entry)
		}
	}
	return env
}
