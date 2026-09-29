package jobs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// Name is a job's directory name: the time it was asked for, then a few words
// of the brief. A person reading `ls` should be able to tell which job is which,
// and the leading timestamp sorts them in the order they were asked.
func Name(brief string, at time.Time) string {
	stamp := at.UTC().Format("20060102-1504")
	words := slug(brief)
	if words == "" {
		return stamp
	}
	return stamp + "-" + words
}

func slug(text string) string {
	var out []rune
	words := 0
	for _, letter := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(letter) || unicode.IsDigit(letter):
			out = append(out, letter)
		case len(out) > 0 && out[len(out)-1] != '-':
			words++
			if words >= 5 || len(out) >= 32 {
				return strings.Trim(string(out), "-")
			}
			out = append(out, '-')
		}
	}
	return strings.Trim(string(out), "-")
}

// Reserve takes a name nothing is using and makes the job's directory, which
// only one caller can do for any one name. Two jobs asked for in the same
// minute in the same words are two jobs, the second with a number.
func (i *Index) Reserve(brief string, at time.Time) (string, error) {
	base := Name(brief, at)
	i.guard.Lock()
	defer i.guard.Unlock()
	if err := i.overQuota(at); err != nil {
		return "", err
	}
	for attempt := 1; ; attempt++ {
		name := base
		if attempt > 1 {
			name = base + "-" + itoa(attempt)
		}
		if _, taken := i.records[name]; taken {
			continue
		}
		err := os.Mkdir(filepath.Join(i.root, name), 0o755)
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
}

func itoa(number int) string {
	if number < 10 {
		return string(rune('0' + number))
	}
	return itoa(number/10) + string(rune('0'+number%10))
}
