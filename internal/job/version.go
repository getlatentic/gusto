package job

import "fmt"

// Version is the version of job.json this code writes and the newest it reads.
// An additive change keeps it; a change to what a field means raises it. The
// schema that describes it is schema/job/v1.json.
const Version = 1

// Readable says whether a reader that knows formats up to known can read a file
// that declares version got. A file with no version predates the field and is
// version 1. A newer version is refused by name rather than read as the old one,
// because a field whose meaning changed would be obeyed wrongly and silently.
func Readable(file string, got, known int) error {
	if got < 0 || got > known {
		return fmt.Errorf("%s is version %d, and this build reads up to version %d: use a haas that knows it", file, got, known)
	}
	return nil
}
