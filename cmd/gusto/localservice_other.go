//go:build !darwin && !linux && !windows

package main

import "errors"

var errNoService = errors.New("gusto cannot make itself a service on this system: run gusto local run yourself")

func installService(home string) error { return errNoService }
func stopService() error               { return errNoService }
func uninstallService() error          { return errNoService }
func keepLog(string) error             { return nil }
