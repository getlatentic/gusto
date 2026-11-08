package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

// storedParameters answers a read from what it holds, after failing the first few, and counts the reads.
type storedParameters struct {
	held  map[string]string
	fails int
	err   error
	reads int
}

func (s *storedParameters) GetParameter(_ context.Context, in *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	s.reads++
	if s.reads <= s.fails {
		return nil, s.err
	}
	if !aws.ToBool(in.WithDecryption) {
		return &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String("ciphertext")}}, nil
	}
	value, found := s.held[aws.ToString(in.Name)]
	if !found {
		return nil, &types.ParameterNotFound{}
	}
	return &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String(value)}}, nil
}

func opening(s *storedParameters) func(context.Context) (parameterReader, error) {
	return func(context.Context) (parameterReader, error) { return s, nil }
}

func TestAParameterIsPrintedDecryptedAndAMissingOneIsAnErrorThatIsNotReadAgain(t *testing.T) {
	stored := &storedParameters{held: map[string]string{"/gusto/poc/cell-secret": "s3cret"}}
	var out bytes.Buffer
	if err := printParameter(context.Background(), opening(stored), "/gusto/poc/cell-secret", &out, 0); err != nil || out.String() != "s3cret\n" {
		t.Errorf("printed %q: %v", out.String(), err)
	}
	out.Reset()
	stored.reads = 0
	if err := printParameter(context.Background(), opening(stored), "/gusto/poc/none", &out, 0); err == nil || !strings.Contains(err.Error(), "no parameter /gusto/poc/none") || out.Len() != 0 {
		t.Errorf("a missing parameter printed %q: %v", out.String(), err)
	}
	if stored.reads != 1 {
		t.Errorf("a parameter that is not there was read %d times", stored.reads)
	}
}

func TestAReadThatFailsAtFirstIsTriedAgainAndOneThatKeepsFailingIsGivenUpOn(t *testing.T) {
	slow := errors.New("canceled, context deadline exceeded")
	stored := &storedParameters{held: map[string]string{"/p": "value"}, fails: paramTries - 1, err: slow}
	var out bytes.Buffer
	if err := printParameter(context.Background(), opening(stored), "/p", &out, 0); err != nil || out.String() != "value\n" || stored.reads != paramTries {
		t.Errorf("a read that worked on its last try printed %q after %d reads: %v", out.String(), stored.reads, err)
	}
	stored = &storedParameters{held: map[string]string{"/p": "value"}, fails: 100, err: slow}
	out.Reset()
	if err := printParameter(context.Background(), opening(stored), "/p", &out, 0); err == nil || !strings.Contains(err.Error(), "could not be read") || stored.reads != paramTries || out.Len() != 0 {
		t.Errorf("a read that never worked was tried %d times and said %v", stored.reads, err)
	}
}

func TestAHostThatMayNotReadAParameterIsToldAtOnce(t *testing.T) {
	stored := &storedParameters{held: map[string]string{"/p": "value"}, fails: 100, err: &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "not allowed"}}
	var out bytes.Buffer
	if err := printParameter(context.Background(), opening(stored), "/p", &out, 0); err == nil || !strings.Contains(err.Error(), "may not read") || stored.reads != 1 {
		t.Errorf("a refusal was read %d times and said %v", stored.reads, err)
	}
}

func TestAPauseBetweenReadsEndsWhenTheContextDoes(t *testing.T) {
	stored := &storedParameters{held: map[string]string{"/p": "value"}, fails: 100, err: errors.New("slow")}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := printParameter(ctx, opening(stored), "/p", &bytes.Buffer{}, time.Hour); err == nil || time.Since(started) > 5*time.Second {
		t.Errorf("the pause outlasted the context: %v after %s", err, time.Since(started))
	}
}
