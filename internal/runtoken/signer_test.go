package runtoken

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// signerOverPipes is a signer at the end of two pipes, as the forwarder reaches its signer process, and a way to end it.
func signerOverPipes(t *testing.T) (Prover, func() error) {
	t.Helper()
	asks, asked := io.Pipe()
	answered, answers := io.Pipe()
	served := make(chan error, 1)
	go func() { served <- ServeSigner(asks, answers) }()
	return SignerProver(asked, answered), func() error {
		asked.Close()
		return <-served
	}
}

func TestATokenTradedWithASignersProofsWorksOnlyWithItsProofs(t *testing.T) {
	issuer, _ := anIssuer(t)
	prove, end := signerOverPipes(t)
	proof, err := prove("POST", exchangeURL, "", issuer.now())
	if err != nil {
		t.Fatal(err)
	}
	token, err := issuer.Exchange("boot", proof, "POST", exchangeURL, secrets())
	if err != nil {
		t.Fatal(err)
	}
	proof, err = prove("POST", callURL, token, issuer.now())
	if err != nil {
		t.Fatal(err)
	}
	if run, err := issuer.Check(token, proof, "POST", callURL); err != nil || run != "run-1" {
		t.Fatalf("a call with the signer's proof gave %q, %v", run, err)
	}
	if _, err := call(t, issuer, aKey(t), token); err == nil {
		t.Error("the token worked with a proof from another key")
	}
	if err := end(); err != nil {
		t.Errorf("the signer ended with %v", err)
	}
}

func TestASignerAnswersARequestItCannotReadAndGoesOn(t *testing.T) {
	var answers bytes.Buffer
	asked := "not json\n" + `{"htm":"POST"}` + "\n" + `{"htm":"POST","htu":"` + callURL + `","iat":1800000000}` + "\n"
	if err := ServeSigner(strings.NewReader(asked), &answers); err != nil {
		t.Fatal(err)
	}
	var given []proofGiven
	for decoder := json.NewDecoder(&answers); decoder.More(); {
		var each proofGiven
		if err := decoder.Decode(&each); err != nil {
			t.Fatal(err)
		}
		given = append(given, each)
	}
	if len(given) != 3 || given[0].Error == "" || given[1].Error == "" || given[2].Proof == "" {
		t.Fatalf("the signer answered %+v", given)
	}
}

func TestAProverWhoseSignerHasEndedSaysSo(t *testing.T) {
	prove, end := signerOverPipes(t)
	if err := end(); err != nil {
		t.Fatal(err)
	}
	if _, err := prove("POST", callURL, "", time.Unix(1_800_000_000, 0)); err == nil {
		t.Error("a proof came from a signer that has ended")
	}
}
