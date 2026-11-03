package changelog

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
)

// repairTail removes a last line with no newline. Append writes each line with its newline in one write, and syncs it
// before it says the event is written, so a line without one is an event no Append finished: a write a crash or a full disk
// cut short. Whole lines are never removed: a log that was cut or changed is still refused. spec/AppendLog.tla is the model.
func repairTail(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	end, err := wholeLinesEnd(file, info.Size())
	if err != nil || end == info.Size() {
		return err
	}
	slog.Warn("the change log ended in part of a line that no write finished, which is removed", "path", path, "bytes", info.Size()-end)
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
