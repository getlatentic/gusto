package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// What one process writes for another to read is read back as it was written, and only its owner can read the file.
func TestASettingsFileWrittenIsReadBackAsItWas(t *testing.T) {
	var e example
	declareExample(New(), &e)
	path := filepath.Join(t.TempDir(), "server.toml")
	written := map[string]string{"GUSTO_ADDR": `127.0.0.1:"8040"\n`, "GUSTO_TASK_CONCURRENCY": "3"}
	if err := WriteFile(path, written); err != nil {
		t.Fatal(err)
	}
	read, err := ReadFile(path)
	if err != nil || len(read) != 2 || read["GUSTO_ADDR"] != written["GUSTO_ADDR"] || read["GUSTO_TASK_CONCURRENCY"] != "3" {
		t.Fatalf("%v %v", read, err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("the file is %v", info.Mode().Perm())
	}
}

// A secret, an old name and a name nothing declares stay out of a settings file.
func TestASettingsFileHoldsNoSecretOldNameOrUnknownSetting(t *testing.T) {
	var e example
	set := New()
	declareExample(set, &e)
	set.Renamed("GUSTO_TASK_CONCURRENCY", "GUSTO_WORKERS")
	for name, holds := range map[string]bool{"GUSTO_ADDR": true, "GUSTO_PASSWORD": false, "GUSTO_WORKERS": false, "GUSTO_NOT_A_SETTING": false} {
		if FileHolds(name) != holds {
			t.Errorf("FileHolds(%s) is %v", name, !holds)
		}
	}
}
