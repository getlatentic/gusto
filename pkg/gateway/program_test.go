package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestMain lets this test program be the MCP server that a test starts as a program: started with GATEWAY_FAKE_SERVER it speaks MCP
// over its standard input and output and does nothing else.
func TestMain(m *testing.M) {
	if mode := os.Getenv("GATEWAY_FAKE_SERVER"); mode != "" {
		fakeProgram(mode)
		return
	}
	os.Exit(m.Run())
}

func fakeProgram(mode string) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	out := json.NewEncoder(os.Stdout)
	if mode == "chatty" {
		fmt.Fprintln(os.Stderr, "starting up, listening on stdio")
		fmt.Fprintln(os.Stdout, `{"jsonrpc":"2.0","method":"notifications/message","params":{"level":"info"}}`)
		// A server may ask its client something.
		out.Encode(map[string]any{"jsonrpc": "2.0", "id": "srv-1", "method": "ping"})
	}
	for scanner.Scan() {
		var in struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &in) != nil || len(in.ID) == 0 || in.Method == "" {
			continue
		}
		if mode == "mute" && in.Method == "initialize" {
			fmt.Fprintln(os.Stderr, "fatal: the license file is missing")
			os.Exit(3)
		}
		var result any
		switch in.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{
				{"name": "echo", "description": "Says back what it is given.", "inputSchema": map[string]any{"type": "object"}},
				{"name": "env", "description": "Says what the program's own environment holds.", "inputSchema": map[string]any{"type": "object"}},
			}}
		case "tools/call":
			var asked struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			json.Unmarshal(in.Params, &asked)
			if mode == "crashes" {
				fmt.Fprintln(os.Stderr, "panic: something broke")
				os.Exit(2)
			}
			said := "echo " + string(asked.Arguments)
			if asked.Name == "env" {
				said = "PLUGIN_ROOT=" + os.Getenv("PLUGIN_ROOT")
			}
			result = map[string]any{"content": []map[string]string{{"type": "text", "text": said}}}
		default:
			out.Encode(map[string]any{"jsonrpc": "2.0", "id": in.ID, "error": map[string]any{"code": -32601, "message": "unknown"}})
			continue
		}
		out.Encode(map[string]any{"jsonrpc": "2.0", "id": in.ID, "result": result})
	}
}

// aProgram is a launcher that runs this test program as the server, in the mode named by the server's command.
func aProgram(started *atomic.Int32) Launcher {
	return func(ctx context.Context, job string, server Server) (*exec.Cmd, func(), error) {
		if started != nil {
			started.Add(1)
		}
		command := exec.Command(os.Args[0])
		command.Env = append(os.Environ(), "GATEWAY_FAKE_SERVER="+server.Command, "PLUGIN_ROOT="+server.Plugin)
		return command, nil, nil
	}
}

func TestAServerThatIsAProgramIsStartedWhenFirstUsedAndSpokenToOverItsStandardInput(t *testing.T) {
	var started atomic.Int32
	h := start(t, Config{Servers: []Server{{Name: "local", Command: "chatty", Plugin: "/plugins/reports"}}, Launch: aProgram(&started)})
	if started.Load() != 0 {
		t.Fatal("a program was started before anything was asked of it")
	}
	found, failed := h.call("tool_search", map[string]any{"query": "echo"})
	if failed || !strings.Contains(found, "local/echo") {
		t.Fatalf("search = %q", found)
	}
	said, failed := h.call("tool_call", map[string]any{"name": "local/echo", "arguments": map[string]any{"a": 1}})
	if failed || said != `echo {"a":1}` {
		t.Errorf("call = %q %v", said, failed)
	}
	if said, _ := h.call("tool_call", map[string]any{"name": "local/env"}); said != "PLUGIN_ROOT=/plugins/reports" {
		t.Errorf("the program's environment: %q", said)
	}
	if started.Load() != 1 {
		t.Errorf("the program was started %d times, once for the job", started.Load())
	}
}

func TestAProgramThatDoesNotStartIsSaidWithWhatItWroteAndTheOtherServersStay(t *testing.T) {
	_, up := reports(t)
	h := start(t, Config{Servers: []Server{{Name: "broken", Command: "mute"}, up}, Launch: aProgram(nil)})
	found, _ := h.call("tool_search", map[string]any{"query": "report"})
	if !strings.Contains(found, "reports/get_report") || !strings.Contains(found, "the server broken could not be reached") || !strings.Contains(found, "the license file is missing") {
		t.Errorf("search = %q", found)
	}
}

func TestAProgramThatDiesInTheMiddleOfACallIsAFailedCallAndNotAHang(t *testing.T) {
	h := start(t, Config{Servers: []Server{{Name: "fragile", Command: "crashes"}}, Launch: aProgram(nil)})
	began := time.Now()
	said, failed := h.call("tool_call", map[string]any{"name": "fragile/echo"})
	if !failed || !strings.Contains(said, "ended before it answered") || !strings.Contains(said, "something broke") {
		t.Errorf("%q %v", said, failed)
	}
	if time.Since(began) > 10*time.Second {
		t.Errorf("the call hung for %v", time.Since(began))
	}
	var call Entry
	for _, entry := range h.entries() {
		if entry.Kind == "call" {
			call = entry
		}
	}
	if call.Outcome != "failed" {
		t.Errorf("the record says %+v", call)
	}
}

func TestAGatewayWithNoLauncherRunsNoProgram(t *testing.T) {
	h := start(t, Config{Servers: []Server{{Name: "local", Command: "chatty"}}})
	found, _ := h.call("tool_search", map[string]any{})
	if !strings.Contains(found, "runs no programs") {
		t.Errorf("search = %q", found)
	}
}

func TestAProgramEndsWithTheJob(t *testing.T) {
	var command *exec.Cmd
	launch := func(ctx context.Context, job string, server Server) (*exec.Cmd, func(), error) {
		made, ended, _ := aProgram(nil)(ctx, job, server)
		command = made
		return made, ended, nil
	}
	h := start(t, Config{Servers: []Server{{Name: "local", Command: "chatty"}}, Launch: launch})
	h.call("tool_search", map[string]any{})
	if command.Process == nil {
		t.Fatal("the program was not started")
	}
	h.session.Close()
	if command.ProcessState == nil {
		t.Error("the program is still running after the job ended")
	}
}
