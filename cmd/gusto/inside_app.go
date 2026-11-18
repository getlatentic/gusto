package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/getlatentic/gusto/internal/runner"
)

// insideApp is what an app's box runs: the app, as the agent, reached through a socket the server outside shares.
func insideApp(args []string) int {
	app, err := insideAppArgs(args)
	if err != nil {
		return usageError("inside-app", err.Error())
	}
	place, code := insidePlace()
	if code != 0 {
		return code
	}
	// The runtime stops a box with a termination, which is handed on to the app.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	exit, err := runner.ServeApp(ctx, app, place)
	if err != nil {
		return fail(err)
	}
	return exit
}

// insideAppArgs reads --socket, --port and --dir, and the app's command, one line, after --.
func insideAppArgs(args []string) (runner.App, error) {
	named, command, err := namedAndCommand(args)
	if err != nil {
		return runner.App{}, err
	}
	port, err := strconv.Atoi(named["--port"])
	app := runner.App{Socket: named["--socket"], Folder: named["--dir"], Port: port, Command: command}
	if err != nil || port <= 0 || app.Socket == "" || app.Folder == "" {
		return runner.App{}, errors.New(insideAppUsage)
	}
	return app, nil
}

const insideAppUsage = "needs --socket <path> --port <port> --dir <folder> -- <command>"

// namedAndCommand is the flags before -- by name, each with its value, and the one argument after it.
func namedAndCommand(args []string) (map[string]string, string, error) {
	named := map[string]string{}
	for len(args) >= 2 && args[0] != "--" {
		switch args[0] {
		case "--socket", "--port", "--dir":
			named[args[0]] = args[1]
		default:
			return nil, "", errors.New("does not know " + args[0])
		}
		args = args[2:]
	}
	if len(args) != 2 || args[0] != "--" || args[1] == "" {
		return nil, "", errors.New(insideAppUsage)
	}
	return named, args[1], nil
}
