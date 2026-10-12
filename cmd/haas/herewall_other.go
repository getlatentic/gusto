//go:build !darwin && !linux

package main

import (
	"errors"
	"os/exec"

	"github.com/getlatentic/haas/internal/job"
)

func wrapProcess(job.Dir, wallPolicy) (func(*exec.Cmd), error) {
	return nil, errors.New("this system has no process sandbox")
}
