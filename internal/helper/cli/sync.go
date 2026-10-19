package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/getlatentic/gusto/internal/helper"
	"github.com/getlatentic/gusto/internal/helper/mirror"
)

// Sync keeps a folder on this computer the same as an agent's folder in the cloud. Files are real files on both sides, and the
// two are compared every few seconds. [mirror] has the rule: a change on one side goes to the other, a change beats a
// deletion, and when both sides changed a file the local one keeps its name and the cloud's is saved beside it.
func Sync(args []string) int {
	flags := flag.NewFlagSet("sync", flag.ContinueOnError)
	address := serverFlag(flags)
	instance := flags.String("instance", os.Getenv("GUSTO_INSTANCE"), "the agent in the cloud whose folder this is (default your newest)")
	folder := flags.String("folder", "", "the folder on this computer")
	once := flags.Bool("once", false, "compare once and stop")
	dry := flags.Bool("dry-run", false, "say what would be done, and do nothing")
	force := flags.Bool("force", false, "go on when the checks say it looks like a mistake")
	interval := flags.Duration("every", 3*time.Second, "how often to compare while there are changes (it waits longer when there are none)")
	if flags.Parse(args) != nil {
		return 2
	}
	server, ok := needServer(*address)
	if !ok {
		return 2
	}
	if *folder == "" {
		fmt.Fprintln(os.Stderr, "name the folder on this computer: -folder ~/work/project")
		return 2
	}
	root, err := filepath.Abs(*folder)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	vault, err := helper.OpenVault(os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	token, err := vault.Get(server)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	client := helper.Client{Server: server, Token: token}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	chosen, err := agentFor(ctx, client, *instance)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	engine, err := newEngine(root, server, chosen, client, *force)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("Keeping %s the same as %s's folder in the cloud.\n", root, firstNonEmpty(chosen.Title, chosen.Harness))
	if *dry {
		return preview(ctx, engine)
	}
	if *once {
		report, err := engine.Once(ctx)
		return said(report, err)
	}
	wake, slow := wakes(ctx, root, engine.Ignore, client, chosen.ID)
	fmt.Println("Compares when a file is saved or a turn changes, and every few seconds while there are changes. Press Ctrl+C to stop.")
	_ = engine.Keep(ctx, *interval, slow, wake, func(report *mirror.Report, err error) {
		if err != nil || (report != nil && (len(report.Errors) > 0 || changed(report))) {
			said(report, err)
		}
	})
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// agentFor is the agent whose folder is kept: the one named, or the person's newest.
func agentFor(ctx context.Context, client helper.Client, named string) (helper.Instance, error) {
	all, err := client.Instances(ctx)
	if err != nil {
		return helper.Instance{}, err
	}
	var picked *helper.Instance
	for i := range all {
		if (named != "" && all[i].ID == named) || (named == "" && (picked == nil || all[i].Started > picked.Started)) {
			picked = &all[i]
		}
	}
	if picked == nil {
		if named != "" {
			return helper.Instance{}, fmt.Errorf("you have no agent called %s", named)
		}
		return helper.Instance{}, fmt.Errorf("you have no agent yet: start one on the web, or with `gusto-helper acp`, and run this again")
	}
	return *picked, nil
}

// newEngine is the engine for one folder and one agent, with its record kept in the helper's settings folder, named by the
// three things that make this one pair: the server, the agent and the folder.
func newEngine(root, server string, agent helper.Instance, client helper.Client, force bool) (*mirror.Engine, error) {
	dir, err := settingsDir()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(server + "\x00" + agent.ID + "\x00" + root))
	state, err := mirror.OpenState(filepath.Join(dir, "sync", hex.EncodeToString(sum[:8])+".json"))
	if err != nil {
		return nil, err
	}
	ignore, err := mirror.ReadIgnore(root)
	if err != nil {
		return nil, err
	}
	for _, line := range ignore.Unsupported {
		fmt.Fprintf(os.Stderr, "%s in %s is not understood and is ignored: a pattern that takes a path back in is not supported\n", line, mirror.IgnoreFile)
	}
	return &mirror.Engine{Root: root, Remote: mirror.Cloud{Client: client, Instance: agent.ID}, State: state, Ignore: ignore, Force: force,
		Log: func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }}, nil
}

func changed(report *mirror.Report) bool {
	for kind, n := range report.Done {
		if n > 0 && kind != mirror.Settle && kind != mirror.Forget {
			return true
		}
	}
	return false
}

// said tells a person what a run did, and gives the exit code.
func said(report *mirror.Report, err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var parts []string
	for _, kind := range []mirror.Kind{mirror.Pull, mirror.Push, mirror.DeleteLocal, mirror.DeleteRemote, mirror.Conflict} {
		if n := report.Done[kind]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, strings.ReplaceAll(string(kind), "-", " ")))
		}
	}
	if len(parts) > 0 {
		fmt.Println(time.Now().Format("15:04:05"), strings.Join(parts, ", "))
	}
	for _, name := range report.Conflicts {
		fmt.Printf("  both sides changed this file: the cloud's version is kept beside yours as %s\n", name)
	}
	if n := len(report.Skipped.Large); n > 0 {
		fmt.Printf("  %d file(s) are over the size limit and are left as they are\n", n)
	}
	if n := len(report.Skipped.Links); n > 0 {
		fmt.Printf("  %d link(s) are not followed\n", n)
	}
	for _, line := range report.Errors {
		fmt.Fprintln(os.Stderr, "  could not:", line)
	}
	if len(report.Errors) > 0 {
		return 1
	}
	return 0
}

func preview(ctx context.Context, engine *mirror.Engine) int {
	report, err := engine.Preview(ctx)
	if report != nil {
		var lines []string
		for _, act := range report.Plan {
			if act.Kind != mirror.Settle && act.Kind != mirror.Forget {
				line := fmt.Sprintf("  %-14s %s", act.Kind, act.Path)
				if act.Copy != "" {
					line += "   (the cloud's version is kept as " + act.Copy + ")"
				}
				lines = append(lines, line)
			}
		}
		sort.Strings(lines)
		if len(lines) == 0 {
			fmt.Println("Nothing to do: the two folders are the same.")
		}
		for _, line := range lines {
			fmt.Println(line)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
