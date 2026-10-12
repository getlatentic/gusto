package keychain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"time"
)

// secretService is the desktop's Secret Service (GNOME Keyring, KWallet), through secret-tool. A key is given to it on
// standard input. A keyring that is locked asks the person to unlock it, so every call has a deadline rather than waiting on
// a prompt nobody may see.
type secretService struct{}

func system() (Store, error) {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return nil, errors.New("secret-tool is not installed (libsecret-tools)")
	}
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" && os.Getenv("XDG_RUNTIME_DIR") == "" {
		return nil, errors.New("there is no desktop session to keep a key in")
	}
	return secretService{}, nil
}

func secretTool(stdin string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "secret-tool", args...)
	command.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil && stderr.Len() > 0 {
		err = fmt.Errorf("%w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out, err
}

func (secretService) Keep(provider, key string) error {
	_, err := secretTool(key, "store", "--label", "haas: "+provider, "service", service, "provider", provider)
	return err
}

func (secretService) Read(provider string) (string, error) {
	out, err := secretTool("", "lookup", "service", service, "provider", provider)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 && len(out) == 0 {
		return "", fmt.Errorf("no key for %s in the keyring: %w", provider, fs.ErrNotExist)
	}
	return string(out), err
}

func (secretService) Forget(provider string) error {
	_, err := secretTool("", "clear", "service", service, "provider", provider)
	return err
}
