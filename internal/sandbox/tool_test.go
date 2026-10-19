package sandbox

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func aTool(t *testing.T) Tool {
	t.Helper()
	root := t.TempDir()
	return Tool{Job: "20261008-1200-a-turn", Name: "reports-db", Plugin: filepath.Join(root, "plugin"), Data: filepath.Join(root, "data"),
		Command: "./bin/server", Args: []string{"--data", "${PLUGIN_DATA}/db", "--config", "${PLUGIN_ROOT}/config.json", "${PLUGIN_ROOTS}"},
		Env: map[string]string{"CONFIG": "${PLUGIN_ROOT}/c.json", "PLUGIN_ROOT": "/elsewhere", "A=B": "x"}, Cwd: "${PLUGIN_DATA}/work"}
}

func TestAProgramsBoxHasNoCapabilityNoNetworkAndNothingItMayWriteButItsOwnFolder(t *testing.T) {
	args, err := ToolArgs(aTool(t))
	if err != nil {
		t.Fatal(err)
	}
	line := " " + strings.Join(args, " ") + " "
	for _, want := range []string{" --network=none ", " --read-only ", " --cap-drop=ALL ", " --security-opt=no-new-privileges ", " --user=65534:65534 ",
		" --pids-limit=128 ", " --memory=512m ", " --cpus=0.5 ", " --init ", ":/plugin:ro ", ":/data ", " --entrypoint /plugin/bin/server "} {
		if !strings.Contains(line, want) {
			t.Errorf("the command line lacks %q:\n%s", want, line)
		}
	}
	for _, never := range []string{"--privileged", "--network=host", "docker.sock", "--cap-add", "/job"} {
		if strings.Contains(line, never) {
			t.Errorf("the command line holds %q", never)
		}
	}
	if args[len(args)-1] != "${PLUGIN_ROOTS}" || !strings.Contains(line, " --data /data/db --config /plugin/config.json ") {
		t.Errorf("placeholders: %s", line)
	}
	if !strings.Contains(line, " -w /data/work ") || !strings.Contains(line, " -e CONFIG=/plugin/c.json ") {
		t.Errorf("cwd and env: %s", line)
	}
	if strings.Contains(line, "/elsewhere") || strings.Contains(line, "A=B") {
		t.Errorf("the package set a variable that is the client's: %s", line)
	}
	if args[4] != "--name" || args[5] != "gusto-tool-20261008-1200-a-turn-reports-db" {
		t.Errorf("name: %v", args[:8])
	}
}

func TestTheNetworkIsOpenOnlyWhenAnAdministratorSaysSo(t *testing.T) {
	if ToolNetwork() != "none" {
		t.Fatal("a program has a network by default")
	}
	t.Setenv("GUSTO_TOOLS_NETWORK", "open")
	args, _ := ToolArgs(aTool(t))
	if !strings.Contains(strings.Join(args, " "), "--network=bridge") {
		t.Errorf("%v", args)
	}
	t.Setenv("GUSTO_TOOLS_NETWORK", "host")
	if ToolNetwork() != "none" {
		t.Error("anything but open is not open")
	}
}

func TestACommandOrWorkingFolderOutsideThePluginIsRefused(t *testing.T) {
	for _, change := range []func(*Tool){
		func(tool *Tool) { tool.Command = "./../../etc/x" },
		func(tool *Tool) { tool.Command = "/usr/bin/x" },
		func(tool *Tool) { tool.Cwd = "/etc" },
		func(tool *Tool) { tool.Cwd = "${PLUGIN_ROOT}/../.." },
	} {
		tool := aTool(t)
		change(&tool)
		if _, err := ToolArgs(tool); err == nil {
			t.Errorf("%+v was taken", tool)
		}
	}
	tool := aTool(t)
	tool.Command = "node"
	if args, err := ToolArgs(tool); err != nil || !strings.Contains(strings.Join(args, " "), "--entrypoint node ") {
		t.Errorf("a bare name: %v %v", args, err)
	}
}

// realToolImage is an image this machine has that carries node, which the real test runs a small MCP server in.
func realToolImage(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath(Runtime()); err != nil {
		t.Skip("no container runtime")
	}
	image := "node:24-slim"
	if err := exec.Command(Runtime(), "image", "inspect", image).Run(); err != nil {
		t.Skip("no " + image + " here")
	}
	return image
}

const echoServer = `
const readline = require("readline");
const fs = require("fs");
const rl = readline.createInterface({ input: process.stdin });
const reply = (id, result) => console.log(JSON.stringify({ jsonrpc: "2.0", id, result }));
rl.on("line", (line) => {
  const m = JSON.parse(line);
  if (m.id === undefined) return;
  if (m.method === "initialize") return reply(m.id, { protocolVersion: "2025-06-18", capabilities: { tools: {} } });
  if (m.method === "tools/list") return reply(m.id, { tools: [{ name: "probe", description: "Says what the box allows.", inputSchema: { type: "object" } }] });
  const said = [];
  const tryIt = (what, fn) => { try { fn(); said.push(what + ":yes"); } catch (e) { said.push(what + ":no"); } };
  tryIt("write-plugin", () => fs.writeFileSync("/plugin/x", "x"));
  tryIt("write-root", () => fs.writeFileSync("/x", "x"));
  tryIt("write-data", () => fs.writeFileSync("/data/x", "x"));
  tryIt("write-tmp", () => fs.writeFileSync("/tmp/x", "x"));
  tryIt("read-job", () => fs.readFileSync("/job/job.json"));
  tryIt("read-docker-socket", () => fs.statSync("/var/run/docker.sock"));
  said.push("nics:" + fs.readdirSync("/sys/class/net").join(","));
  said.push("uid:" + process.getuid());
  said.push("root:" + (process.env.PLUGIN_ROOT || ""));
  reply(m.id, { content: [{ type: "text", text: said.join(" ") }] });
});
`

func TestAProgramRunsInARealBoxThatHoldsItInAndTheGatewayCanSpeakToIt(t *testing.T) {
	image := realToolImage(t)
	t.Setenv("GUSTO_TOOLS_IMAGE", image)
	// A folder under the home directory, which a runtime that is a virtual machine shares and a temporary folder it may not.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	root, err := os.MkdirTemp(home, ".gusto-tool-test-")
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	plugin, data := filepath.Join(root, "plugin"), filepath.Join(root, "data")
	os.MkdirAll(plugin, 0o755)
	os.WriteFile(filepath.Join(plugin, "server.js"), []byte(echoServer), 0o644)
	tool := Tool{Job: "20261008-1200-real", Name: "probe", Plugin: plugin, Data: data, Command: "node", Args: []string{"${PLUGIN_ROOT}/server.js"}}
	command, ended, err := ToolCommand(context.Background(), tool)
	if err != nil {
		t.Fatal(err)
	}
	defer ended()
	var failed strings.Builder
	command.Stderr = &failed
	stdin, _ := command.StdinPipe()
	stdout, _ := command.StdoutPipe()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	lines := bufio.NewReader(stdout)
	ask := func(line string) string {
		stdin.Write([]byte(line + "\n"))
		answer, _ := lines.ReadString('\n')
		return answer
	}
	ask(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	got := ask(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"probe"}}`)
	for _, want := range []string{"write-plugin:no", "write-root:no", "write-data:yes", "write-tmp:yes", "read-job:no", "read-docker-socket:no", "uid:65534", "root:/plugin", "nics:lo "} {
		if !strings.Contains(got, want) {
			t.Errorf("the box did not say %q:\n%s\nstderr: %s", want, got, failed.String())
		}
	}
	if _, err := os.Stat(filepath.Join(data, "x")); err != nil {
		t.Errorf("what the program wrote to its own folder is not there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(plugin, "x")); err == nil {
		t.Error("the program changed the plugin")
	}
}
