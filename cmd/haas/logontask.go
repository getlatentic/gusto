package main

import (
	"bytes"
	"encoding/xml"
	"unicode/utf16"
)

// logonTask is the Task Scheduler's definition of haas for one person on Windows. Its trigger is that person's own logon,
// which a person may register without being an administrator, where schtasks /SC ONLOGON is any logon and needs one. It
// runs on battery, is never stopped for running long, is started again if it fails, and runs with no window: conhost
// --headless gives a console program a console nobody sees.
func logonTask(who, program string) []byte {
	text := func(s string) string {
		var out bytes.Buffer
		xml.EscapeText(&out, []byte(s))
		return out.String()
	}
	task := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>haas, the agents on this computer</Description></RegistrationInfo>
  <Triggers><LogonTrigger><Enabled>true</Enabled><UserId>` + text(who) + `</UserId></LogonTrigger></Triggers>
  <Principals><Principal id="Author"><UserId>` + text(who) + `</UserId><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <RestartOnFailure><Interval>PT1M</Interval><Count>3</Count></RestartOnFailure>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author"><Exec><Command>conhost.exe</Command><Arguments>--headless "` + text(program) + `" local run</Arguments></Exec></Actions>
</Task>
`
	// schtasks reads a definition that says it is UTF-16 as UTF-16, little-endian, after its byte order mark.
	out := []byte{0xff, 0xfe}
	for _, unit := range utf16.Encode([]rune(task)) {
		out = append(out, byte(unit), byte(unit>>8))
	}
	return out
}
