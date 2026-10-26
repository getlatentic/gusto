package cell

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// EC2 starts and stops machines for hosts with the AWS command, which uses the role of the host it runs on. The hosts
// are made from a launch template, tagged so that only these are ever listed or stopped, and given the start-up
// script that makes them hosts of this front door.
type EC2 struct {
	// Template is the launch template's name.
	Template string
	// UserData is the start-up script with its @NAME@ settings, and Settings what they are to be.
	UserData string
	Settings map[string]string
	// Project is the value of the project tag, which says whose hosts they are. It is gusto-poc when not set.
	Project string
	// Run runs the AWS command and gives what it printed. It is exec by default, and a test gives its own.
	Run func(args ...string) ([]byte, error)
}

var instanceID = regexp.MustCompile(`^i-[0-9a-f]{8,17}$`)

// A new host's disk is read from the image's snapshot a block at a time, and the template gives it throughput to be read in many
// streams: a start-up on the baseline took 41 s, with the first read of a parameter timing out, against 8.5 s. That is billed while a
// host is stopped too, so a kept host's disk, once it has been read, is lowered to the baseline when the host is first stopped.
const (
	baselineIOPS       = "3000"
	baselineThroughput = 125
)

var volumeID = regexp.MustCompile(`^vol-[0-9a-f]{8,17}$`)

// awsCall bounds one AWS command, so a front door whose network to AWS hangs still decides again.
const awsCall = 2 * time.Minute

func (e *EC2) project() string {
	if e.Project != "" {
		return e.Project
	}
	return "gusto-poc"
}

func (e *EC2) run(args ...string) ([]byte, error) {
	if e.Run != nil {
		return e.Run(args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), awsCall)
	defer cancel()
	out, err := exec.CommandContext(ctx, "aws", args...).Output()
	var failed *exec.ExitError
	if errors.As(err, &failed) {
		return out, fmt.Errorf("aws %s: %s", args[1], strings.TrimSpace(string(failed.Stderr)))
	}
	return out, err
}

// script is the start-up script with the settings put in.
func (e *EC2) script() (string, error) {
	text, err := os.ReadFile(e.UserData)
	if err != nil {
		return "", err
	}
	out := string(text)
	for name, value := range e.Settings {
		out = strings.ReplaceAll(out, "@"+name+"@", value)
	}
	if left := regexp.MustCompile(`@[A-Z_]+@`).FindString(out); left != "" {
		return "", fmt.Errorf("the start-up script has %s and nothing says what it is", left)
	}
	return out, nil
}

// scriptFile writes the start-up script to a file the AWS command reads, and says how to remove it.
func (e *EC2) scriptFile(encode bool) (string, func(), error) {
	script, err := e.script()
	if err != nil {
		return "", nil, err
	}
	if encode {
		script = base64.StdEncoding.EncodeToString([]byte(script))
	}
	file, err := os.CreateTemp("", "gusto-cell-userdata-")
	if err != nil {
		return "", nil, err
	}
	remove := func() { os.Remove(file.Name()) }
	if _, err := file.WriteString(script); err != nil {
		file.Close()
		remove()
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		remove()
		return "", nil, err
	}
	return file.Name(), remove, nil
}

// Launch starts a new host. A kept one is stopped rather than removed when it is idle, and started again when a host is needed.
func (e *EC2) Launch(kept bool) (string, error) {
	path, remove, err := e.scriptFile(false)
	if err != nil {
		return "", err
	}
	defer remove()
	tags := "{Key=project,Value=" + e.project() + "},{Key=name,Value=demo-cell},{Key=scaler,Value=on}"
	args := []string{"ec2", "run-instances", "--launch-template", "LaunchTemplateName=" + e.Template + ",Version=$Latest", "--user-data", "file://" + path}
	if kept {
		tags += ",{Key=kept,Value=on}"
	}
	args = append(args, "--tag-specifications", "ResourceType=instance,Tags=["+tags+"]", "--query", "Instances[0].InstanceId", "--output", "text")
	out, err := e.run(args...)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(out))
	if !instanceID.MatchString(id) {
		return "", fmt.Errorf("AWS did not give a host: %q", id)
	}
	return id, nil
}

// Start starts a stopped kept host with today's start-up script, which it runs on every start, so it comes up as this front
// door's release.
func (e *EC2) Start(id string) error {
	if !instanceID.MatchString(id) {
		return fmt.Errorf("%q is not a host", id)
	}
	// The AWS command does not encode the value of modify-instance-attribute, and EC2 wants it encoded.
	path, remove, err := e.scriptFile(true)
	if err != nil {
		return err
	}
	defer remove()
	if _, err := e.run("ec2", "modify-instance-attribute", "--instance-id", id, "--attribute", "userData", "--value", "file://"+path); err != nil {
		return err
	}
	_, err = e.run("ec2", "start-instances", "--instance-ids", id)
	return err
}

// Stop stops a kept host and lowers its root disk to the baseline. Its own disk, where its sandboxes were, is blanked by AWS; its root
// disk is kept. A disk that cannot be lowered now is lowered at a later stop, so that does not fail the stop.
func (e *EC2) Stop(id string) error {
	if !instanceID.MatchString(id) {
		return fmt.Errorf("%q is not a host", id)
	}
	if _, err := e.run("ec2", "stop-instances", "--instance-ids", id); err != nil {
		return err
	}
	if err := e.lowerDisk(id); err != nil {
		slog.Warn("a stopped host's disk could not be lowered to the baseline", "host", id, "error", err)
	}
	return nil
}

// lowerDisk takes a host's root disk down to the baseline, once: a disk already there is left alone, since a disk may be changed only
// every six hours.
func (e *EC2) lowerDisk(id string) error {
	out, err := e.run("ec2", "describe-instances", "--instance-ids", id,
		"--query", "Reservations[0].Instances[0].BlockDeviceMappings[0].Ebs.VolumeId", "--output", "text")
	if err != nil {
		return err
	}
	volume := strings.TrimSpace(string(out))
	if !volumeID.MatchString(volume) {
		return fmt.Errorf("AWS did not give a disk: %q", volume)
	}
	out, err = e.run("ec2", "describe-volumes", "--volume-ids", volume, "--query", "Volumes[0].Throughput", "--output", "text")
	if err != nil {
		return err
	}
	if have, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil && have <= baselineThroughput {
		return nil
	}
	_, err = e.run("ec2", "modify-volume", "--volume-id", volume, "--iops", baselineIOPS, "--throughput", strconv.Itoa(baselineThroughput))
	return err
}

func (e *EC2) Terminate(id string) error {
	if !instanceID.MatchString(id) {
		return fmt.Errorf("%q is not a host", id)
	}
	_, err := e.run("ec2", "terminate-instances", "--instance-ids", id)
	return err
}

// Running is the scaler's hosts that are starting or up. A started host's launch time is when it was last started.
func (e *EC2) Running() ([]Instance, error) { return e.listed("pending,running") }

// Stopped is the scaler's hosts that are stopping or stopped, which only kept hosts are.
func (e *EC2) Stopped() ([]Instance, error) { return e.listed("stopping,stopped") }

func (e *EC2) listed(states string) ([]Instance, error) {
	out, err := e.run("ec2", "describe-instances",
		"--filters", "Name=tag:scaler,Values=on", "Name=tag:project,Values="+e.project(), "Name=instance-state-name,Values="+states,
		"--query", "Reservations[].Instances[].[InstanceId,LaunchTime,State.Name,Tags[?Key=='kept']|[0].Value]", "--output", "text")
	if err != nil {
		return nil, err
	}
	var hosts []Instance
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 || !instanceID.MatchString(fields[0]) {
			continue
		}
		launched, err := time.Parse(time.RFC3339, fields[1])
		if err != nil {
			continue
		}
		hosts = append(hosts, Instance{ID: fields[0], Launched: launched, Stopped: fields[2] == "stopped", Kept: fields[3] == "on"})
	}
	return hosts, nil
}
