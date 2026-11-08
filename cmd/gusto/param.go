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
	"github.com/aws/smithy-go"
)

// A read of Parameter Store, credentials included, is tried up to paramTries times, each bounded by paramCall, so a read that is
// slow when a host has just started, from a disk still being fetched, is tried again and not given up on.
const (
	paramCall  = 8 * time.Second
	paramTries = 3
	paramPause = time.Second
)

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
	if err := printParameter(context.Background(), openParameters, args[0], os.Stdout, paramPause); err != nil {
		return fail(err)
	}
	return 0
}

// openParameters is Parameter Store as the AWS configuration finds it: an instance role, single sign-on, or the environment.
func openParameters(ctx context.Context) (parameterReader, error) {
	loaded, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("the AWS configuration could not be read: %w", err)
	}
	return ssm.NewFromConfig(loaded), nil
}

// printParameter prints the parameter, trying again after a pause when a read fails in a way that another might not: a parameter
// that is not there, or that this host may not read, is answered at once.
func printParameter(ctx context.Context, open func(context.Context) (parameterReader, error), name string, out io.Writer, pause time.Duration) error {
	var err error
	for try := 1; try <= paramTries; try++ {
		var value string
		if value, err = readParameter(ctx, open, name); err == nil {
			_, err = fmt.Fprintln(out, value)
			return err
		}
		if final(err) || try == paramTries {
			break
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(pause):
		}
	}
	return err
}

func readParameter(ctx context.Context, open func(context.Context) (parameterReader, error), name string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, paramCall)
	defer cancel()
	api, err := open(ctx)
	if err != nil {
		return "", err
	}
	got, err := api.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(name), WithDecryption: aws.Bool(true)})
	var missing *types.ParameterNotFound
	if errors.As(err, &missing) {
		return "", &answered{fmt.Errorf("there is no parameter %s", name)}
	}
	var refused smithy.APIError
	if errors.As(err, &refused) && refused.ErrorCode() == "AccessDeniedException" {
		return "", &answered{fmt.Errorf("this host may not read the parameter %s: %w", name, err)}
	}
	if err != nil {
		return "", fmt.Errorf("the parameter %s could not be read: %w", name, err)
	}
	if got.Parameter == nil || got.Parameter.Value == nil {
		return "", &answered{fmt.Errorf("the parameter %s has no value", name)}
	}
	return *got.Parameter.Value, nil
}

// answered is an error that another read would not change.
type answered struct{ error }

func (a *answered) Unwrap() error { return a.error }

func final(err error) bool {
	var done *answered
	return errors.As(err, &done)
}
