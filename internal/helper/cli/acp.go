package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/getlatentic/gusto/internal/helper"
	"github.com/getlatentic/gusto/internal/helper/bridge"
)

// signedInCloud is the person's organization, reached with the token the keychain holds at the moment of each call, so a
// person who signs in after their editor started the helper needs to start nothing again.
type signedInCloud struct {
	server string
	vault  helper.Vault
}

func (c signedInCloud) client() (helper.Client, error) {
	token, err := c.vault.Get(c.server)
	if err != nil {
		return helper.Client{}, err
	}
	return helper.Client{Server: c.server, Token: token}, nil
}

func (c signedInCloud) Instances(ctx context.Context) ([]helper.Instance, error) {
	client, err := c.client()
	if err != nil {
		return nil, err
	}
	return client.Instances(ctx)
}

func (c signedInCloud) Engines(ctx context.Context) ([]helper.Engine, error) {
	client, err := c.client()
	if err != nil {
		return nil, err
	}
	return client.Engines(ctx)
}

func (c signedInCloud) StartInstance(ctx context.Context, harness, model string) (helper.Instance, helper.Thread, error) {
	client, err := c.client()
	if err != nil {
		return helper.Instance{}, helper.Thread{}, err
	}
	return client.StartInstance(ctx, harness, model)
}

func (c signedInCloud) NewThread(ctx context.Context, instance, title string) (helper.Thread, error) {
	client, err := c.client()
	if err != nil {
		return helper.Thread{}, err
	}
	return client.NewThread(ctx, instance, title)
}

func (c signedInCloud) Say(ctx context.Context, instance, thread, said, key string) (helper.Said, error) {
	client, err := c.client()
	if err != nil {
		return helper.Said{}, err
	}
	return client.Say(ctx, instance, thread, said, key)
}

func (c signedInCloud) FollowJob(ctx context.Context, job string, told helper.Follower) (helper.Ended, error) {
	client, err := c.client()
	if err != nil {
		return helper.Ended{}, err
	}
	return client.FollowJob(ctx, job, told)
}

func (c signedInCloud) StopJob(ctx context.Context, job string) error {
	client, err := c.client()
	if err != nil {
		return err
	}
	return client.StopJob(ctx, job)
}

// ACP is the helper as an agent for an editor that speaks the Agent Client Protocol. The editor starts it and talks to it on
// standard input and output, which are the protocol's and carry nothing else: whatever the helper has to say goes to standard
// error, where an editor shows it in its log.
func ACP(args []string) int {
	flags := flag.NewFlagSet("acp", flag.ContinueOnError)
	address := serverFlag(flags)
	instance := flags.String("instance", os.Getenv("GUSTO_INSTANCE"), "the agent in the cloud to work with (default your newest, or a new one)")
	engine := flags.String("agent", os.Getenv("GUSTO_AGENT_KIND"), "the kind of agent to start when you have none (default the first this server can run)")
	if flags.Parse(args) != nil {
		return 2
	}
	server, ok := needServer(*address)
	if !ok {
		return 2
	}
	vault, err := helper.OpenVault(os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	agent := &bridge.Agent{Cloud: signedInCloud{server: server, vault: vault}, Instance: *instance, Engine: *engine, Version: Version, Log: os.Stderr}
	if err := agent.Run(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// Version is what the helper says its release is, set when it is built with -ldflags "-X .../internal/helper/cli.Version=<tag>".
var Version = "dev"
