package cell

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// EC2 starts and stops hosts for cells with the AWS command, which uses the role of the host it runs on. The hosts
// are made from a launch template, tagged so that only these are ever listed or stopped, and given the start-up
// script that makes them cells of this front door.
type EC2 struct {
	// Template is the launch template's name.
	Template string
	// UserData is the start-up script with its @NAME@ settings, and Settings what they are to be.
	UserData string
	Settings map[string]string
	// Run runs the AWS command and gives what it printed. It is exec by default, and a test gives its own.
	Run func(args ...string) ([]byte, error)
}

var instanceID = regexp.MustCompile(`^i-[0-9a-f]{8,17}$`)

func (e *EC2) run(args ...string) ([]byte, error) {
	if e.Run != nil {
		return e.Run(args...)
	}
	out, err := exec.Command("aws", args...).Output()
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

func (e *EC2) Launch() (string, error) {
	script, err := e.script()
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "haas-cell-userdata-")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(script); err != nil {
		file.Close()
		return "", err
	}
	file.Close()
	out, err := e.run("ec2", "run-instances",
		"--launch-template", "LaunchTemplateName="+e.Template+",Version=$Latest",
		"--user-data", "file://"+file.Name(),
		"--tag-specifications", "ResourceType=instance,Tags=[{Key=project,Value=haas-poc},{Key=name,Value=demo-cell},{Key=scaler,Value=on}]",
		"--query", "Instances[0].InstanceId", "--output", "text")
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(out))
	if !instanceID.MatchString(id) {
		return "", fmt.Errorf("AWS did not give a host: %q", id)
	}
	return id, nil
}

func (e *EC2) Terminate(id string) error {
	if !instanceID.MatchString(id) {
		return fmt.Errorf("%q is not a host", id)
	}
	_, err := e.run("ec2", "terminate-instances", "--instance-ids", id)
	return err
}

func (e *EC2) Running() ([]Instance, error) {
	out, err := e.run("ec2", "describe-instances",
		"--filters", "Name=tag:scaler,Values=on", "Name=tag:project,Values=haas-poc", "Name=instance-state-name,Values=pending,running",
		"--query", "Reservations[].Instances[].[InstanceId,LaunchTime]", "--output", "text")
	if err != nil {
		return nil, err
	}
	var hosts []Instance
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !instanceID.MatchString(fields[0]) {
			continue
		}
		launched, err := time.Parse(time.RFC3339, fields[1])
		if err != nil {
			continue
		}
		hosts = append(hosts, Instance{ID: fields[0], Launched: launched})
	}
	return hosts, nil
}
