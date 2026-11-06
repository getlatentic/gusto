package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/doctor"
	"github.com/getlatentic/gusto/internal/settings"
	"github.com/getlatentic/gusto/internal/signing"
)

// checkMachine checks this machine before a task is sent to it, and exits 1 when
// anything a task needs is missing, so a deploy can stop on it. A computer with
// gusto installed for its person, and no settings of a server in the shell, is
// checked as that install runs.
func checkMachine() int {
	lookup, err := checkedSettings()
	if err != nil {
		return refused(err)
	}
	var checked doctorSettings
	set, err := checked.load(lookup)
	if err != nil {
		return refused(err)
	}
	where := indexesOf(checked.stores)
	results := append(doctor.Run(doctor.Checks(doctor.This(), checked.forChecks(set))), usageCheck(where), taskIndexCheck(where))
	doctor.Print(os.Stdout, results)
	if !doctor.Healthy(results) {
		return 1
	}
	return 0
}

// checkedSettings is the settings of the server to check: this shell's, or those of the install this computer has for its person.
func checkedSettings() (func(string) string, error) {
	home, err := localHome()
	if err != nil || !checksLocalInstall(home, os.Getenv, fileExists) || os.Chdir(home) != nil {
		return os.Getenv, nil
	}
	decided := map[string]string{}
	if setup, err := ensureLocal(home); err == nil {
		decided["GUSTO_PASSWORD"] = setup.Secret
	}
	adoptRecordedPath()
	own, err := ownComputer(home)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Checking the gusto installed for you, in %s.\n\n", home)
	return settings.Over(decided, own), nil
}

// doctorSettings is what gusto doctor reads of the server it checks.
type doctorSettings struct {
	core      coreSettings
	stores    storeSettings
	boxes     sandboxSettings
	models    wayOutSettings
	harnesses string
}

func (d *doctorSettings) declare(set *settings.Set) {
	d.core.declare(set)
	d.stores.declare(set)
	d.boxes.declare(set)
	d.models.declareModels(set)
	set.String(&d.harnesses, "GUSTO_HARNESSES", "", "the directory of agents of your own, beside the built-in ones")
}

func (d *doctorSettings) load(lookup func(string) string) (*settings.Set, error) {
	set := settings.New()
	d.declare(set)
	return set, set.Load(lookup)
}

// forChecks is the settings as the checks take them. The model is checked only when one is named, and the sandbox is the kind
// decided on, or nothing for the strongest this computer can make.
func (d doctorSettings) forChecks(set *settings.Set) doctor.Settings {
	checked := doctor.Settings{
		Sandbox:    d.boxes.isolation(set.Raw("GUSTO_SANDBOX")),
		Containers: d.boxes.containers.signedBy(signing.Identity{}),
		Jobs:       d.stores.jobs, Harnesses: d.harnesses, Password: d.core.password != "",
		OpenShellRemote: d.boxes.openShell.cfg.Remote,
	}
	if set.Raw("GUSTO_MODEL_URL") != "" {
		checked.ModelURL = d.models.modelURL
	}
	return checked
}

// checksLocalInstall says whether doctor should look at the install this computer has for its person: there is one, and the shell
// names no server's folders or address, so what would be checked is not what the person's install runs.
func checksLocalInstall(home string, getenv func(string) string, exists func(string) bool) bool {
	if !exists(filepath.Join(home, "local.json")) {
		return false
	}
	for _, name := range []string{"GUSTO_JOBS", "GUSTO_ADDR", "GUSTO_SANDBOX", "GUSTO_CONTAINER"} {
		if getenv(name) != "" {
			return false
		}
	}
	return true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
