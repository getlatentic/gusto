package changelog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
)

// repairTail removes a last line with no newline. Append writes each line with its newline in one write, and syncs it
// before it says the event is written, so a line without one is an event no Append finished: a write a crash or a full disk
// cut short. Whole lines are never removed: a log that was cut or changed is still refused. spec/AppendLog.tla is the model.
//
// The log is read to find its end, and opened to be cut only when a line is torn: where it is append-only, as a deploy makes
// it, a whole log opens as before, and a torn one says what to do.
func repairTail(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	end, err := wholeLinesEnd(file, info.Size())
	file.Close()
	if err != nil || end == info.Size() {
		return err
	}
	slog.Warn("the change log ended in part of a line that no write finished, which is removed", "path", path, "bytes", info.Size()-end)
	return cutTail(path, end)
}

func cutTail(path string, end int64) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("the change log ends in part of a line no write finished, and cannot be cut back to its whole lines; "+
			"where it is append-only, run chattr -a on it and start the server, which cuts it, then deploy to make it append-only again: %w", err)
	}
	defer file.Close()
	if err := file.Truncate(end); err != nil {
		return err
	}
	return file.Sync()
}

// wholeLinesEnd is where the file's last newline ends, reading back from the end a block at a time, so a long log is not
// read whole.
func wholeLinesEnd(file io.ReaderAt, size int64) (int64, error) {
	const block = 64 << 10
	buf := make([]byte, block)
	for end := size; end > 0; end -= block {
		start := max(end-block, 0)
		chunk := buf[:end-start]
		if _, err := file.ReadAt(chunk, start); err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		if at := bytes.LastIndexByte(chunk, '\n'); at >= 0 {
			return start + int64(at) + 1, nil
		}
	}
	return 0, nil
}
