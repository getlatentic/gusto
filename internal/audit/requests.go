package audit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strings"
)

// requestsNaming keeps the lines of a request log whose path names the task,
// byte for byte as the log wrote them.
//
// The path is the only thing that ties a request to a task. The request that
// created the task is not among them: a task is created by POST /jobs or POST
// /api/jobs, and its ID is chosen by the server after the path was written.
// What remains are the reads: the task's page, its files, its bundle and its
// stream, which is who looked at the record.
func requestsNaming(log, id string) ([]byte, error) {
	text, err := os.ReadFile(log)
	if err != nil {
		return nil, err
	}
	var kept bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if names(scanner.Bytes(), id) {
			kept.Write(scanner.Bytes())
			kept.WriteByte('\n')
		}
	}
	return kept.Bytes(), scanner.Err()
}

// names reads one line the way internal/accesslog writes it. A line that is
// not a request, such as anything else journald kept, names nothing.
func names(line []byte, id string) bool {
	var request struct {
		Msg  string `json:"msg"`
		Path string `json:"path"`
	}
	if json.Unmarshal(line, &request) != nil || request.Msg != "request" {
		return false
	}
	for _, segment := range strings.Split(request.Path, "/") {
		if segment == id {
			return true
		}
	}
	return false
}
