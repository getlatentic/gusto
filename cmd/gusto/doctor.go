package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/doctor"
)

// checkMachine checks this machine before a task is sent to it, and exits 1 when
// anything a task needs is missing, so a deploy can stop on it. A computer with
// gusto installed for its person, and no settings of a server in the shell, is
// checked as that install runs.
func checkMachine() int {
	if home, err := localHome(); err == nil && checksLocalInstall(home, os.Getenv, fileExists) {
		if err := os.Chdir(home); err == nil {
			if setup, err := ensureLocal(home); err == nil {
				os.Setenv("GUSTO_PASSWORD", setup.Secret)
			}
			adoptRecordedPath()
			useInstalledWall(home)
			chooseWall()
			fmt.Printf("Checking the gusto installed for you, in %s.\n\n", home)
		}
	}
	results := append(doctor.Run(doctor.Checks(doctor.This())), usageCheck(), taskIndexCheck())
	doctor.Print(os.Stdout, results)
	if !doctor.Healthy(results) {
		return 1
	}
	return 0
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
