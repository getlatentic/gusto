package main

import (
	"os"

	"github.com/getlatentic/gusto/internal/doctor"
)

// checkMachine checks this machine before a task is sent to it, and exits 1 when
// anything a task needs is missing, so a deploy can stop on it.
func checkMachine() int {
	results := doctor.Run(doctor.Checks(doctor.This()))
	doctor.Print(os.Stdout, results)
	if !doctor.Healthy(results) {
		return 1
	}
	return 0
}
