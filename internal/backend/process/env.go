package process

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// sharedCache is where an agent run here keeps what it downloads, such as the packages an agent installs into its own
// configuration: one folder for every task, so a turn does not download them again.
func sharedCache() string {
	if cache, err := os.UserCacheDir(); err == nil {
		return filepath.Join(cache, "gusto")
	}
	return filepath.Join(os.TempDir(), "gusto-cache")
}

// homedEnv is env with the agent's home and data put where a program looks for them, so an agent reads the configuration
// gusto gave it and not the person's own, and its downloads in cache.
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
