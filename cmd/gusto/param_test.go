package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type storedParameters map[string]string

func (s storedParameters) GetParameter(_ context.Context, in *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	if !aws.ToBool(in.WithDecryption) {
		return &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String("ciphertext")}}, nil
	}
	value, found := s[aws.ToString(in.Name)]
	if !found {
		return nil, &types.ParameterNotFound{}
	}
	return &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String(value)}}, nil
}

func TestAParameterIsPrintedDecryptedAndAMissingOneIsAnError(t *testing.T) {
	stored := storedParameters{"/gusto/poc/cell-secret": "s3cret"}
	var out bytes.Buffer
	if err := printParameter(context.Background(), stored, "/gusto/poc/cell-secret", &out); err != nil || out.String() != "s3cret\n" {
		t.Errorf("printed %q: %v", out.String(), err)
	}
	out.Reset()
	if err := printParameter(context.Background(), stored, "/gusto/poc/none", &out); err == nil || !strings.Contains(err.Error(), "no parameter /gusto/poc/none") || out.Len() != 0 {
		t.Errorf("a missing parameter printed %q: %v", out.String(), err)
	}
}
