package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// paramCall bounds one read of Parameter Store, credentials included.
const paramCall = 20 * time.Second

// parameterReader is the part of Parameter Store a host reads its secrets with.
type parameterReader interface {
	GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// paramCommand prints one parameter, decrypted. A host reads its secrets with it while it starts, where the AWS command, a Python
// program, took five seconds to start from a disk that is still being fetched, and this takes a fraction of one.
func paramCommand(args []string) int {
	if len(args) != 1 || args[0] == "" {
		return usageError("param", "needs the name of one parameter")
	}
	ctx, cancel := context.WithTimeout(context.Background(), paramCall)
	defer cancel()
	loaded, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fail(fmt.Errorf("the AWS configuration could not be read: %w", err))
	}
	if err := printParameter(ctx, ssm.NewFromConfig(loaded), args[0], os.Stdout); err != nil {
		return fail(err)
	}
	return 0
}

func printParameter(ctx context.Context, api parameterReader, name string, out io.Writer) error {
	got, err := api.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(name), WithDecryption: aws.Bool(true)})
	var missing *types.ParameterNotFound
	if errors.As(err, &missing) {
		return fmt.Errorf("there is no parameter %s", name)
	}
	if err != nil {
		return fmt.Errorf("the parameter %s could not be read: %w", name, err)
	}
	if got.Parameter == nil || got.Parameter.Value == nil {
		return fmt.Errorf("the parameter %s has no value", name)
	}
	_, err = fmt.Fprintln(out, *got.Parameter.Value)
	return err
}
