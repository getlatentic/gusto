package proxy

import (
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/wire"
)

func TestGemmaIsKnownAsAChatOnlyProviderThatIsTranslatedTo(t *testing.T) {
	gemma, found := Known("gemma")
	if !found || gemma.Name != "gemma" || gemma.Key != "" || gemma.MaxOutput != 8192 {
		t.Fatalf("gemma: %+v %v", gemma, found)
	}
	if gemma.Answers(wire.Messages) || !gemma.Answers(wire.Chat) || !gemma.Translates(wire.Messages) {
		t.Error("gemma answers Chat Completions alone, and a Messages call to it is translated")
	}
	if held := gemma.WithKey("k"); held.Key != "k" || gemma.Key != "" {
		t.Error("WithKey changed the provider it was called on, or did not set the key")
	}
	gemma.Wires[0] = "messages"
	if again, _ := Known("gemma"); again.Wires[0] != "chat" {
		t.Error("a caller changed what Known returns for everybody")
	}
}

func TestAnUnknownProviderIsNotKnown(t *testing.T) {
	if _, found := Known("nobody"); found {
		t.Error("a provider nobody named is known")
	}
}

// The addresses are written once more, in the script that makes a deployment's credential files.
func TestTheKnownProvidersAreTheOnesTheDeploymentWrites(t *testing.T) {
	script, err := os.ReadFile("../../deploy/aws/openshell/e2e/credentials.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range KnownNames() {
		credential, _ := Known(name)
		for _, address := range credential.Addresses() {
			if !strings.Contains(string(script), `"address": "`+address+`"`) {
				t.Errorf("%s at %s is not what e2e/credentials.sh writes", name, address)
			}
		}
	}
}
