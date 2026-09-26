package main

import (
	"os"

	"github.com/getlatentic/haas/internal/doctor"
)

// checkMachine checks this machine before a job is sent to it, and exits 1 when
// anything a job needs is missing, so a deploy can stop on it.
func checkMachine(args []string) int {
	if len(args) != 0 {
		usage()
		return 2
	}
	results := doctor.Run(doctor.Checks(doctor.This()))
	doctor.Print(os.Stdout, results)
	if !doctor.Healthy(results) {
		return 1
	}
	return 0
}
