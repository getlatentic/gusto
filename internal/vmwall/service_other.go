//go:build !darwin && !linux

package vmwall

import "errors"

var errNoService = errors.New("virtual machines for jobs are not supported on this system")

func startGateway(Layout) error { return errNoService }
func stopGateway(Layout) error  { return nil }
