package usage

import "time"

func NewRecorderSized(store Store, who WhoFunc, provider ProviderFunc, queue int, retry time.Duration) *Recorder {
	return newRecorder(store, who, provider, queue, retry)
}

func Scrub(err error) string { return scrub(err) }

func Lose(r *Recorder, tasks ...string) { r.markLost(tasks...) }
