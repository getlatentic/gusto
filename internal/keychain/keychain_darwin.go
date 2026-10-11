package keychain

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
)

// macKeychain is the person's login keychain, through security(1). A key is given to it on standard input, in its
// interactive mode, since an argument is readable by every user of the computer.
type macKeychain struct{}

func system() (Store, error) { return macKeychain{}, nil }

func (macKeychain) Keep(provider, key string) error {
	if strings.ContainsAny(provider+key, "\"\\\r\n") {
		return errors.New("the keychain is given a key with no quote, backslash or line break in it")
	}
	command := exec.Command("/usr/bin/security", "-i")
	command.Stdin = strings.NewReader(fmt.Sprintf("add-generic-password -U -s %q -a %q -w %q\n", service, provider, key))
	if out, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

func (macKeychain) Read(provider string) (string, error) {
	out, err := exec.Command("/usr/bin/security", "find-generic-password", "-s", service, "-a", provider, "-w").Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 44 {
		return "", fmt.Errorf("no key for %s in the keychain: %w", provider, fs.ErrNotExist)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func (macKeychain) Forget(provider string) error {
	err := exec.Command("/usr/bin/security", "delete-generic-password", "-s", service, "-a", provider).Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 44 {
		return nil
	}
	return err
}
