package instance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// lift reads a conversation written before an instance had threads and writes
// it out in the shape that has them: the workspace it always was, and the one
// thread it always had.
//
// The old file is left where it is. Once the new one is beside it nothing reads
// the old one again, and a deploy that has to be rolled back still finds it.
func lift(dir string) (Instance, error) {
	text, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		return Instance{}, err
	}
	var was struct {
		ID         string   `json:"id"`
		Harness    string   `json:"harness"`
		Model      string   `json:"model"`
		Repo       string   `json:"repo"`
		Remembered string   `json:"remembered"`
		Turns      []string `json:"turns"`
		Started    int64    `json:"startedAt"`
	}
	if err := json.Unmarshal(text, &was); err != nil {
		return Instance{}, err
	}
	held := Instance{
		ID: was.ID, Harness: was.Harness, Model: was.Model,
		Repo: was.Repo, Started: was.Started,
	}
	if err := os.MkdirAll(filepath.Join(dir, "threads"), 0o755); err != nil {
		return Instance{}, err
	}
	thread := Thread{
		ID: "1", Instance: was.ID, Title: "the first conversation",
		Remembered: was.Remembered, Turns: was.Turns, Started: was.Started,
	}
	if thread.Started == 0 {
		thread.Started = time.Now().Unix()
	}
	if err := writeJSON(filepath.Join(dir, "threads", "1.json"), thread); err != nil {
		return Instance{}, err
	}
	return held, writeJSON(filepath.Join(dir, "instance.json"), held)
}

func writeJSON(name string, value any) error {
	text, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(name, append(text, '\n'), 0o644)
}
