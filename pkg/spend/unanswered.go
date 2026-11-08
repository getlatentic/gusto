package spend

import "fmt"

// Silence says a run's model never answered it: every call the run made to a model came back refused or broken.
type Silence struct {
	// Calls is how many calls to a model the run made.
	Calls int `json:"calls"`
	// Reason is what went wrong with the last of them, as the proxy saw it.
	Reason string `json:"reason"`
}

// Unanswered reads the task's proxy record for a run that started at since, and is the silence when the run asked a model at
// least once and was never answered, or nil. A call a spending cap refused is the cap's story, told by Stopped, and is not
// counted here. What the agent made of the silence is not read: the proxy's record is the word on what the model did.
func Unanswered(path string, since int64) (*Silence, error) {
	calls, err := ReadCalls(path)
	if err != nil {
		return nil, err
	}
	var silence Silence
	for _, call := range calls {
		if !call.ToModel() || call.Refused != nil || call.AtUnix < since {
			continue
		}
		if call.answered() {
			return nil, nil
		}
		silence.Calls++
		silence.Reason = failureOf(call)
	}
	if silence.Calls == 0 {
		return nil, nil
	}
	return &silence, nil
}

func failureOf(call Call) string {
	if call.Error != "" {
		return call.Error
	}
	return fmt.Sprintf("the model's provider answered %s with status %d", call.Path, call.Status)
}
