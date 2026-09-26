package jobs

import (
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

// Free returns a name nothing is using yet, adding a number if it has to.
func (i *Index) Free(brief string, at time.Time) string {
	base := Name(brief, at)
	i.guard.Lock()
	defer i.guard.Unlock()
	name := base
	for attempt := 2; ; attempt++ {
		if _, taken := i.records[name]; !taken {
			return name
		}
		name = base + "-" + itoa(attempt)
	}
}

func itoa(number int) string {
	if number < 10 {
		return string(rune('0' + number))
	}
	return itoa(number/10) + string(rune('0'+number%10))
}
