package taskindex

import "regexp"

var secrets = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(://[^:/@\s]*:)[^@\s]+@`),
	regexp.MustCompile(`(?i)(password\s*=\s*)('[^']*'|\S+)`),
}

// Scrub is an error's text with any password in a database URL or connection string taken out.
func Scrub(err error) string {
	text := err.Error()
	text = secrets[0].ReplaceAllString(text, "${1}***@")
	return secrets[1].ReplaceAllString(text, "${1}***")
}
