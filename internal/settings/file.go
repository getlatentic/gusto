package settings

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// FileEnv names the settings file. Every setting but a secret may be written there as its key (Key). The file fills in the
// process's environment where it has nothing, so a setting in the environment is read over the same one in the file, and
// everything that reads a setting, a child process too, reads the same value.
const FileEnv = "GUSTO_SETTINGS_FILE"

// Key is a setting's name in a settings file: its name without GUSTO_, in lower case, so GUSTO_CLOUD_URL is cloud_url.
func Key(name string) string { return strings.ToLower(strings.TrimPrefix(name, "GUSTO_")) }

// nameOf is the setting a key in a settings file is.
func nameOf(key string) string { return "GUSTO_" + strings.ToUpper(key) }

var keyShape = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// known is every setting the program has declared, in any of its commands, which help declares them all for. One file may serve
// every gusto process on a host, so a key one command does not read is still right when another does; a key none reads is
// a mistake.
var known = struct {
	sync.Mutex
	secret  map[string]bool   // by name, whether it is a secret
	renamed map[string]string // an old name, to the name it has now
	removed map[string]string // a name no longer read, to what does its job
}{secret: map[string]bool{}, renamed: map[string]string{}, removed: map[string]string{}}

func remember(e *entry) {
	known.Lock()
	defer known.Unlock()
	known.secret[e.name] = known.secret[e.name] || e.secret
	if e.was != "" {
		known.renamed[e.was] = e.name
	}
}

func forget(name, instead string) {
	known.Lock()
	defer known.Unlock()
	known.removed[name] = instead
}

// ApplyFile reads the settings file lookup names, if any, and sets each setting in it that lookup has nothing for. lookup and
// set are os.Getenv and os.Setenv for a command. What the file may not hold stops the command, each by key: a secret, which
// only the environment carries; an old name; a key no setting has; and a value of a kind no setting is.
func ApplyFile(lookup func(name string) string, set func(name, value string) error) error {
	path := lookup(FileEnv)
	if path == "" {
		return nil
	}
	values, err := readFile(path)
	if err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(values)) {
		if lookup(name) != "" {
			continue
		}
		if err := set(name, values[name]); err != nil {
			return err
		}
	}
	return nil
}

// readFile is the settings a file holds, by name, each as the text it would be in the environment.
func readFile(path string) (map[string]string, error) {
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, fmt.Errorf("%s is %q, and it could not be read: %w", FileEnv, path, err)
	}
	values := make(map[string]string, len(raw))
	var problems []error
	for _, key := range slices.Sorted(maps.Keys(raw)) {
		text, err := fileValue(key, raw[key])
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %s %w", path, key, err))
			continue
		}
		values[nameOf(key)] = text
	}
	return values, errors.Join(problems...)
}

// fileValue is a key's value in a settings file as text, or why the file may not hold it.
func fileValue(key string, value any) (string, error) {
	if !keyShape.MatchString(key) {
		return "", errors.New("is not a setting's key, which is lower case letters, digits and _")
	}
	if err := fileMay(nameOf(key)); err != nil {
		return "", err
	}
	return asText(value)
}

// fileMay says why a settings file may not hold a setting: one that is secret, that was renamed, or that nothing declares.
func fileMay(name string) error {
	known.Lock()
	defer known.Unlock()
	if instead, gone := known.removed[name]; gone {
		return fmt.Errorf("is no longer read: %s", instead)
	}
	if now, old := known.renamed[name]; old {
		return fmt.Errorf("is now called %s", Key(now))
	}
	secret, declared := known.secret[name]
	switch {
	case !declared:
		return errors.New("is not a setting")
	case secret:
		return fmt.Errorf("is a secret, which is read from the environment only, as %s", name)
	}
	return nil
}

// asText is a value in a settings file as the text it would be in the environment: a list is one comma-separated line.
func asText(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case bool:
		return strconv.FormatBool(v), nil
	case []any:
		return listText(v)
	}
	return "", errors.New("is a string, a whole number, true or false, or a list of strings")
}

func listText(members []any) (string, error) {
	texts := make([]string, 0, len(members))
	for _, member := range members {
		text, ok := member.(string)
		if !ok || strings.Contains(text, ",") {
			return "", errors.New("is a list of strings with no commas in them")
		}
		texts = append(texts, text)
	}
	return strings.Join(texts, ","), nil
}
