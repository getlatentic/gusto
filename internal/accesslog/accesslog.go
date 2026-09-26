// Package accesslog records who asked the server for what: one structured line
// a request, written when the answer is finished.
//
// It needs nothing from the pages it wraps. Whether a guard admitted the caller
// is already in the status: a 303 to /login or a 401 is a refusal, anything else
// was let through.
package accesslog

import (
	"log/slog"
	"net/http"
	"os"
	"time"
)

// Stderr is a logger that writes JSON to standard error, where journald keeps
// what a service prints.
func Stderr() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, nil))
}

// Keep wraps a handler so every request it answers leaves a line in logger.
func Keep(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if unremarkable(r) {
				next.ServeHTTP(w, r)
				return
			}
			started := time.Now()
			answer := &recorder{ResponseWriter: w}
			// The line is written after the handler returns, so a stream is
			// recorded once, when it ends, with everything it sent.
			next.ServeHTTP(answer, r)
			write(logger, r, answer, time.Since(started))
		})
	}
}

// unremarkable leaves out /healthz. A liveness probe polls it on a timer and
// always hears the same answer, so its lines would bury the ones that say who
// asked for a job, and none of them would name anybody.
func unremarkable(r *http.Request) bool {
	return r.URL.Path == "/healthz"
}

// write names the path and never the query string, and names the kind of
// credential and never its value, so the log is safe to hand to whoever reads
// journald.
func write(logger *slog.Logger, r *http.Request, answer *recorder, took time.Duration) {
	logger.LogAttrs(r.Context(), slog.LevelInfo, "request",
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.Int("status", answer.status()),
		slog.Int64("bytes", answer.bytes),
		slog.Float64("duration_ms", float64(took.Microseconds())/1000),
		slog.String("remote", remote(r)),
		slog.String("caller", caller(r)),
	)
}
