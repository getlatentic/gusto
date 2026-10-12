//go:build !darwin && !linux && !windows

package keychain

import "errors"

func system() (Store, error) {
	return nil, errors.New("haas keeps no key in a keychain on this system")
}
