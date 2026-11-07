//go:build !darwin && !linux

package process

import (
	"errors"
	"os/exec"

	"github.com/getlatentic/gusto/pkg/task"
)

func wrapProcess(task.Dir, wallPolicy) (func(*exec.Cmd), error) {
	return nil, errors.New("this system has no process sandbox")
}
