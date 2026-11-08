package isolation

import "testing"

func TestAProcessSandboxChosenBySettingIsNamedForTheSystemItIsOn(t *testing.T) {
	for goos, want := range map[string]string{"darwin": "Seatbelt", "linux": "Landlock"} {
		host := Host{GOOS: goos, Settings: Settings{Sandbox: "process"}}
		if got := Choose(host); got.Kind != Process || got.Detail != want || !got.Forced {
			t.Errorf("%s: chose %+v, want a forced process sandbox named %s", goos, got, want)
		}
	}
}
