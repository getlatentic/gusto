//go:build !darwin && !linux && !windows

package main

import "errors"

var errNoService = errors.New("haas cannot make itself a service on this system: run haas local run yourself")

func installService(home string) error { return errNoService }
func stopService() error               { return errNoService }
func uninstallService() error          { return errNoService }
