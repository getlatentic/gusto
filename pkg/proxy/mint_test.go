package proxy

import "testing"

func TestATaskMintedAgainKeepsItsTokenAndTakesTheNewCredential(t *testing.T) {
	tokens := NewTokens()
	first, err := tokens.Mint("a-job", "openai")
	if err != nil {
		t.Fatal(err)
	}
	again, err := tokens.Mint("a-job", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Error("a task minted again was given another token")
	}
	if ticket, _ := tokens.Task(first); ticket.Credential != "anthropic" {
		t.Errorf("the token is good for %q, not the credential the task was given now", ticket.Credential)
	}
	tokens.Drop("a-job")
	if _, found := tokens.Task(first); found {
		t.Error("a dropped task's token still stands for it")
	}
}
