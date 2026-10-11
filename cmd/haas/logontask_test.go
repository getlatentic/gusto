package main

import (
	"encoding/xml"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestTheLogonTaskIsThePersonsOwnAndKeepsRunning(t *testing.T) {
	raw := logonTask(`ORG-WIN11\o&rg`, `C:\Users\o&rg\AppData\Local\haas\haas.exe`)
	if raw[0] != 0xff || raw[1] != 0xfe || len(raw)%2 != 0 {
		t.Fatal("the definition is not UTF-16, little-endian, with its byte order mark")
	}
	units := make([]uint16, 0, len(raw)/2-1)
	for i := 2; i < len(raw); i += 2 {
		units = append(units, uint16(raw[i])|uint16(raw[i+1])<<8)
	}
	text := string(utf16.Decode(units))
	var task struct {
		Trigger   string `xml:"Triggers>LogonTrigger>UserId"`
		Principal struct {
			UserId, LogonType, RunLevel string
		} `xml:"Principals>Principal"`
		Settings struct {
			DisallowStartIfOnBatteries, StopIfGoingOnBatteries bool
			ExecutionTimeLimit                                 string
		}
		Command   string `xml:"Actions>Exec>Command"`
		Arguments string `xml:"Actions>Exec>Arguments"`
	}
	decoder := xml.NewDecoder(strings.NewReader(strings.Replace(text, `encoding="UTF-16"`, `encoding="UTF-8"`, 1)))
	if err := decoder.Decode(&task); err != nil {
		t.Fatalf("%v\n%s", err, text)
	}
	if task.Trigger != `ORG-WIN11\o&rg` || task.Principal.UserId != task.Trigger || task.Principal.LogonType != "InteractiveToken" || task.Principal.RunLevel != "LeastPrivilege" {
		t.Errorf("the task is not the person's own: %+v %q", task.Principal, task.Trigger)
	}
	if task.Settings.DisallowStartIfOnBatteries || task.Settings.StopIfGoingOnBatteries || task.Settings.ExecutionTimeLimit != "PT0S" {
		t.Errorf("the task stops: %+v", task.Settings)
	}
	if task.Command != "conhost.exe" || task.Arguments != `--headless "C:\Users\o&rg\AppData\Local\haas\haas.exe" local run` {
		t.Errorf("the task runs %s %s", task.Command, task.Arguments)
	}
}
