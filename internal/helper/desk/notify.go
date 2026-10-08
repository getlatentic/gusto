package desk

import (
	"os"
	"os/exec"
	"runtime"
)

// Note is what a notification says.
type Note struct {
	Title string
	Body  string
}

// Notify shows a notification, and says nothing when the system has no way to: the tray still lists the turn.
func Notify(note Note) {
	_ = notifyCommand(runtime.GOOS, note).Start()
}

// notifyCommand gives the text to the program as arguments or environment and never inside a script, so that a brief which
// holds quotes or markup stays text.
func notifyCommand(system string, note Note) *exec.Cmd {
	switch system {
	case "darwin":
		command := exec.Command("osascript",
			"-e", "on run argv",
			"-e", "display notification (item 1 of argv) with title (item 2 of argv)",
			"-e", "end run",
			note.Body, note.Title)
		return command
	case "windows":
		command := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", toast)
		command.Env = append(os.Environ(), "HAAS_NOTE_TITLE="+note.Title, "HAAS_NOTE_BODY="+note.Body)
		return command
	default:
		return exec.Command("notify-send", "--app-name=Latentic Agent Cloud", "--", note.Title, note.Body)
	}
}

const toast = `
$null = [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]
$null = [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime]
$xml = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$text = $xml.GetElementsByTagName('text')
$null = $text.Item(0).AppendChild($xml.CreateTextNode($env:HAAS_NOTE_TITLE))
$null = $text.Item(1).AppendChild($xml.CreateTextNode($env:HAAS_NOTE_BODY))
$id = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($id).Show([Windows.UI.Notifications.ToastNotification]::new($xml))
`
