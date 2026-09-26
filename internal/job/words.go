package job

import "strings"

// Words splits a gate written as one line into the command and its arguments,
// keeping quoted runs together. A gate is a command, and people write
// `sh -c "test $(cat answer) = yes"` — splitting that on spaces produces
// something that cannot run.
func Words(line string) []string {
	var words []string
	var word strings.Builder
	var quote rune
	started := false
	for _, letter := range line {
		switch {
		case quote != 0 && letter == quote:
			quote = 0
		case quote == 0 && (letter == '\'' || letter == '"'):
			quote = letter
			started = true
		case quote == 0 && (letter == ' ' || letter == '\t' || letter == '\n'):
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(letter)
			started = true
		}
	}
	if started {
		words = append(words, word.String())
	}
	return words
}
