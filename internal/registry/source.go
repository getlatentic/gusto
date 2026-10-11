package registry

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Sources reads "stored" keys from a directory and the other kinds from where they say. Only the kinds an
// operator allows are read; the rest are refused when a provider is added and again here.
type Sources struct {
	Dir string
	// Allowed are the kinds besides "stored".
	Allowed []string
	// Run runs a command and gives what it printed. It is exec by default, and a test gives its own.
	Run func(name string, args ...string) ([]byte, error)
}

var referenceName = regexp.MustCompile(`^[A-Za-z0-9_./:@+=-]{1,512}$`)

// Kind is the kind of reference: the part before the colon.
func Kind(reference string) string {
	kind, _, _ := strings.Cut(reference, ":")
	return kind
}

// Check says whether a reference is well formed and of a kind that is allowed.
func Check(reference string, allowed []string) error {
	if reference == "stored" {
		return nil
	}
	kind, name, found := strings.Cut(reference, ":")
	switch {
	case !found || name == "" || !referenceName.MatchString(name) || strings.HasPrefix(name, "-"):
		return errors.New("a key is kept here (stored), or named as file:<path>, ssm:<name> or secretsmanager:<id>")
	case !slices.Contains([]string{"file", "ssm", "secretsmanager"}, kind):
		return fmt.Errorf("%q is not a place a key is read from", kind)
	case !slices.Contains(allowed, kind):
		return fmt.Errorf("this deployment does not read keys from %s", kind)
	case kind == "file" && !filepath.IsAbs(name):
		return errors.New("a file is named by its whole path")
	}
	return nil
}

func (s Sources) Read(reference string) (string, error) {
	if err := Check(reference, s.Allowed); err != nil {
		return "", err
	}
	if reference == "stored" {
		return "", errors.New("a stored key is read by the provider's name")
	}
	kind, name, _ := strings.Cut(reference, ":")
	run := s.Run
	if run == nil {
		run = func(name string, args ...string) ([]byte, error) { return exec.Command(name, args...).Output() }
	}
	var key []byte
	var err error
	switch kind {
	case "file":
		key, err = os.ReadFile(name)
	case "ssm":
		key, err = run("aws", "ssm", "get-parameter", "--with-decryption", "--name", name, "--query", "Parameter.Value", "--output", "text")
	case "secretsmanager":
		key, err = run("aws", "secretsmanager", "get-secret-value", "--secret-id", name, "--query", "SecretString", "--output", "text")
	}
	if err != nil {
		return "", fmt.Errorf("the key in %s could not be read", reference)
	}
	return clean(key)
}

func clean(key []byte) (string, error) {
	text := strings.TrimSpace(string(key))
	if text == "" || len(text) > 4096 || strings.ContainsAny(text, "\r\n\x00") {
		return "", errors.New("that is not a key")
	}
	return text, nil
}

func (r *Registry) keep(provider, key string) error {
	text, err := clean([]byte(key))
	if err != nil {
		return err
	}
	return r.keys.Keep(provider, text)
}

func (r *Registry) forget(provider string) { r.keys.Forget(provider) }

// read is a provider's key, wherever it is.
func (r *Registry) read(p Provider) (string, error) {
	if p.Key == "stored" {
		key, err := r.keys.Read(p.Name)
		if errors.Is(err, fs.ErrNotExist) {
			return "", errors.New("no key has been given for it")
		}
		return key, err
	}
	return Sources{Allowed: r.Rules().KeySources, Run: r.run}.Read(p.Key)
}
