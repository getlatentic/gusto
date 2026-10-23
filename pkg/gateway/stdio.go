package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Launcher makes the command that runs a server that is a program. It is given the task's name and the server, and returns a command
// that has not been started and that does not end with a request, since the program lives as long as the task does, and what is to be
// done when the program has ended, which may be nothing. What it makes is what keeps the program out of the machine: the gateway speaks
// to whatever the command does, and puts nothing around it.
type Launcher func(ctx context.Context, job string, server Server) (command *exec.Cmd, ended func(), err error)

const (
	mostLine     = 8 << 20
	tailOfErrors = 4 << 10
	startTime    = 60 * time.Second
	stopTime     = 3 * time.Second
)

// Program is a connection to a server that is a program, which it speaks to over the program's standard input and output, one JSON
// message to a line. It starts the program the first time it is used and ends it with the task.
type Program struct {
	server Server
	job    string
	launch Launcher
	ids    atomic.Int64

	mu      sync.Mutex
	command *exec.Cmd
	ended   func()
	stdin   io.WriteCloser
	errors  *tail
	waiting map[int64]chan reply
	exited  chan struct{}
	ready   bool
	failed  error
}

type reply struct {
	result json.RawMessage
	err    error
}

// StartProgram makes the connection for a server that is a program.
func StartProgram(server Server, job string, launch Launcher) *Program {
	return &Program{server: server, job: job, launch: launch, waiting: map[int64]chan reply{}}
}

// Tools is every tool the program has.
func (p *Program) Tools(ctx context.Context) ([]Tool, error) {
	return listTools(ctx, p.server.Name, p.call)
}

// Call calls one tool.
func (p *Program) Call(ctx context.Context, tool string, arguments json.RawMessage) (Result, error) {
	return callTool(ctx, p.server.Name, p.call, tool, arguments)
}

// Close ends the program: it is told by closing its input, and killed when it does not go.
func (p *Program) Close() {
	p.mu.Lock()
	command, ended, stdin, exited := p.command, p.ended, p.stdin, p.exited
	p.mu.Unlock()
	if command == nil {
		return
	}
	stdin.Close()
	select {
	case <-exited:
	case <-time.After(stopTime):
		_ = command.Process.Kill()
		<-exited
	}
	if ended != nil {
		ended()
	}
}

func (p *Program) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if err := p.start(ctx); err != nil {
		return nil, err
	}
	return p.exchange(ctx, method, params)
}

func (p *Program) start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failed != nil {
		return p.failed
	}
	if p.ready {
		return nil
	}
	if p.launch == nil {
		p.failed = errors.New("this gateway runs no programs")
		return p.failed
	}
	command, ended, err := p.launch(context.Background(), p.job, p.server)
	if err != nil {
		p.failed = err
		return err
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		p.failed = err
		return err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		p.failed = err
		return err
	}
	p.errors = &tail{limit: tailOfErrors}
	command.Stderr = p.errors
	if err := command.Start(); err != nil {
		p.failed = fmt.Errorf("%s could not be started: %w", p.server.Name, err)
		return p.failed
	}
	p.command, p.ended, p.stdin, p.exited = command, ended, stdin, make(chan struct{})
	go p.read(stdout, command)
	p.mu.Unlock()
	err = p.handshake(ctx)
	p.mu.Lock()
	if err != nil {
		p.failed = fmt.Errorf("%s did not start: %w%s", p.server.Name, err, p.errors.words())
		stdin.Close()
		_ = command.Process.Kill()
		return p.failed
	}
	p.ready = true
	return nil
}

func (p *Program) handshake(ctx context.Context) error {
	ctx, stop := context.WithTimeout(ctx, startTime)
	defer stop()
	init := map[string]any{"protocolVersion": protocols[0], "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "gusto-gateway", "version": "1"}}
	if _, err := p.exchange(ctx, "initialize", init); err != nil {
		return err
	}
	return p.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
}

func (p *Program) send(message any) error {
	line, err := json.Marshal(message)
	if err != nil {
		return err
	}
	p.mu.Lock()
	stdin := p.stdin
	p.mu.Unlock()
	_, err = stdin.Write(append(line, '\n'))
	return err
}

func (p *Program) exchange(ctx context.Context, method string, params any) (json.RawMessage, error) {
	ctx, stop := context.WithTimeout(ctx, callTime)
	defer stop()
	id := p.ids.Add(1)
	answer := make(chan reply, 1)
	p.mu.Lock()
	p.waiting[id] = answer
	exited := p.exited
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.waiting, id); p.mu.Unlock() }()
	message := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		message["params"] = params
	}
	if err := p.send(message); err != nil {
		return nil, fmt.Errorf("%s did not take the message: %w%s", p.server.Name, err, p.errors.words())
	}
	select {
	case got := <-answer:
		return got.result, got.err
	case <-exited:
		return nil, fmt.Errorf("%s ended before it answered%s", p.server.Name, p.errors.words())
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// read takes what the program writes, a message to a line, and gives each answer to the call that waits for it. A program may also
// ask something of its client: a ping is answered, and anything else is told this client does not do it.
func (p *Program) read(stdout io.Reader, command *exec.Cmd) {
	defer func() {
		_ = command.Wait()
		close(p.exited)
	}()
	reader := bufio.NewReaderSize(stdout, 64<<10)
	for {
		line, err := readLine(reader)
		if err != nil {
			return
		}
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if json.Unmarshal(line, &message) != nil {
			continue
		}
		if message.Method != "" {
			if len(message.ID) > 0 {
				p.refuse(message.ID, message.Method)
			}
			continue
		}
		var id int64
		if json.Unmarshal(message.ID, &id) != nil {
			continue
		}
		p.mu.Lock()
		answer := p.waiting[id]
		p.mu.Unlock()
		if answer == nil {
			continue
		}
		got := reply{result: message.Result}
		if message.Error != nil {
			got.err = message.Error
		}
		answer <- got
	}
}

func (p *Program) refuse(id json.RawMessage, method string) {
	if method == "ping" {
		_ = p.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{}})
		return
	}
	_ = p.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": codeMethod, "message": "this client does not do " + method}})
}

// readLine reads one line, of at most mostLine bytes.
func readLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, isPrefix, err := reader.ReadLine()
		if err != nil {
			return nil, err
		}
		line = append(line, part...)
		if len(line) > mostLine {
			return nil, errors.New("a line longer than the gateway reads")
		}
		if !isPrefix {
			return bytes.TrimSpace(line), nil
		}
	}
}

// tail keeps the last of what a program wrote to its error output, to say why it failed.
type tail struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (t *tail) Write(given []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data = append(t.data, given...)
	if len(t.data) > t.limit {
		t.data = t.data[len(t.data)-t.limit:]
	}
	return len(given), nil
}

// words is the end of the error output as a sentence to follow another, or nothing.
func (t *tail) words() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	text := strings.TrimSpace(string(t.data))
	if text == "" {
		return ""
	}
	if i := strings.LastIndex(strings.TrimSpace(text), "\n"); i >= 0 && len(text) > 300 {
		text = text[i+1:]
	}
	return " (it said: " + oneLine(text) + ")"
}
