package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/getlatentic/gusto/internal/landlock"
	"github.com/getlatentic/gusto/internal/runtoken"
)

// An agent in a sandbox reaches its model through a forwarder of the run's own, and is held to a Landlock policy when its task
// says so. gusto boxed is what the sandbox runs in place of the agent: it starts the forwarder, which makes the run's key and
// trades the run's secret for a token bound to it, points the agent at the forwarder, holds itself to the policy, and becomes
// the agent. The forwarder is a process of its own, so the key is never in the agent's memory, and it ends with the agent.

// forwarderStart bounds starting the forwarder, which is one exchange with a proxy on the same network.
const forwarderStart = 30 * time.Second

func boxed(args []string) int {
	model, policy, command, err := boxedArgs(args)
	if err != nil {
		return usageError("boxed", err.Error())
	}
	secret := os.Getenv("GUSTO_MODEL_TOKEN")
	if secret == "" {
		return fail(errors.New("the run has no secret to trade for its token"))
	}
	runtime.LockOSThread()
	local, err := startForwarder(model, secret)
	if err != nil {
		return fail(fmt.Errorf("the run's way to its model did not start, and the agent is not run without it: %w", err))
	}
	program, err := exec.LookPath(command[0])
	if err != nil {
		return fail(err)
	}
	if policy != "" {
		held, err := landlock.Load(policy)
		if err == nil {
			err = landlock.Apply(held)
		}
		if err != nil {
			return fail(fmt.Errorf("the sandbox did not start, and the task is not run unprotected: %w", err))
		}
	}
	swap := strings.NewReplacer(runtoken.Place, local, secret, "dpop")
	return fail(syscall.Exec(program, swapped(command, swap), swapped(os.Environ(), swap)))
}

// boxedArgs reads --model URL, an optional --policy, and the command after --.
func boxedArgs(args []string) (model, policy string, command []string, err error) {
	for len(args) > 0 && args[0] != "--" {
		if len(args) < 2 {
			return "", "", nil, errors.New("needs --model <proxy> [--policy <json>] -- <command>")
		}
		switch args[0] {
		case "--model":
			model = args[1]
		case "--policy":
			policy = args[1]
		default:
			return "", "", nil, fmt.Errorf("%s is not something it takes", args[0])
		}
		args = args[2:]
	}
	if model == "" || len(args) < 2 {
		return "", "", nil, errors.New("needs --model <proxy> [--policy <json>] -- <command>")
	}
	return model, policy, args[1:], nil
}

func swapped(list []string, swap *strings.Replacer) []string {
	out := make([]string, len(list))
	for i, each := range list {
		out[i] = swap.Replace(each)
	}
	return out
}

// startForwarder starts the run's forwarder on a port of this machine's loopback and says its address. The forwarder is handed
// the listening socket, the secret on its standard input, and one end of a pipe that stays open in the agent: when the agent and
// everything it started have ended, the pipe closes and the forwarder ends with it.
func startForwarder(model, secret string) (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	socket, err := listener.(*net.TCPListener).File()
	listener.Close()
	if err != nil {
		return "", err
	}
	defer socket.Close()
	life, keep, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer life.Close()
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	child := exec.Command(self, "boxed-forward", model)
	child.Stdin = strings.NewReader(secret + "\n")
	child.ExtraFiles = []*os.File{socket, life}
	child.Env = []string{"PATH=" + os.Getenv("PATH")}
	child.SysProcAttr = endsWithParent()
	said, err := child.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := child.Start(); err != nil {
		return "", err
	}
	// The agent keeps the pipe open from here: made inheritable only now, so the forwarder does not hold its own end of it.
	if err := inheritable(keep); err != nil {
		child.Process.Kill()
		return "", err
	}
	line, err := firstLine(said, forwarderStart)
	if err != nil || line != "ok" {
		child.Process.Kill()
		return "", fmt.Errorf("the forwarder said %q: %v", line, err)
	}
	return "http://" + listener.Addr().String(), nil
}

func firstLine(from io.Reader, within time.Duration) (string, error) {
	got := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(from).ReadString('\n')
		got <- strings.TrimSpace(line)
	}()
	select {
	case line := <-got:
		return line, nil
	case <-time.After(within):
		return "", errors.New("it did not start in time")
	}
}

// boxedForward is the forwarder: the socket is file 3 and the pipe it lives by is file 4.
func boxedForward(args []string) int {
	hideMemory()
	if len(args) != 1 {
		return usageError("boxed-forward", "needs the proxy's address")
	}
	secret, err := bufio.NewReader(io.LimitReader(os.Stdin, 4<<10)).ReadString('\n')
	if err != nil {
		fmt.Println("no secret")
		return 1
	}
	forwarder, err := runtoken.NewForwarder(args[0], strings.TrimSpace(secret))
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	listener, err := net.FileListener(os.NewFile(3, "socket"))
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	go func() {
		io.Copy(io.Discard, os.NewFile(4, "life"))
		os.Exit(0)
	}()
	fmt.Println("ok")
	server := &http.Server{Handler: forwarder, ReadHeaderTimeout: 30 * time.Second}
	return fail(server.Serve(listener))
}
