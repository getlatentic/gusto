package changelog

import "errors"

// HashOf lets a test do what a forger would: recompute a line's hash.
func HashOf(e Event) (string, error) { return hashOf(e) }

// CutShort does what a write a full disk cuts short does: part of a line reaches the file, and the write fails.
func (l *Log) CutShort(part []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.file.Write(part); err != nil {
		return err
	}
	return l.rollback(errors.New("no space left on device"))
}

// LoseFile closes the file under the log, so a write fails and cannot be cut back.
func (l *Log) LoseFile() error { return l.file.Close() }
