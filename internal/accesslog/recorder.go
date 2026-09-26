package accesslog

import "net/http"

// recorder is the ResponseWriter a handler writes to, noting the status and the
// bytes on their way to the real one.
//
// A stream depends on two things passing straight through it. Flushing reaches
// the connection through FlushError, so a watcher that stopped reading is still
// found as an error rather than hidden behind a Flush that cannot report one.
// Unwrap lets http.ResponseController find the real writer, so a per-frame write
// deadline still lands on the connection.
type recorder struct {
	http.ResponseWriter
	code  int
	bytes int64
}

func (a *recorder) WriteHeader(code int) {
	if a.code == 0 {
		a.code = code
	}
	a.ResponseWriter.WriteHeader(code)
}

func (a *recorder) Write(body []byte) (int, error) {
	a.promise()
	written, err := a.ResponseWriter.Write(body)
	a.bytes += int64(written)
	return written, err
}

func (a *recorder) Flush() {
	_ = a.FlushError()
}

func (a *recorder) FlushError() error {
	a.promise()
	return http.NewResponseController(a.ResponseWriter).Flush()
}

func (a *recorder) Unwrap() http.ResponseWriter {
	return a.ResponseWriter
}

// promise notes the 200 net/http sends by itself when a handler writes or
// flushes before it names a status.
func (a *recorder) promise() {
	if a.code == 0 {
		a.code = http.StatusOK
	}
}

// status is 200 for a handler that wrote nothing at all, because that is what
// net/http answers for it.
func (a *recorder) status() int {
	a.promise()
	return a.code
}
