package cloud

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAMailWithALineBreakInAHeaderIsNotSent(t *testing.T) {
	out := Outbox{Dir: t.TempDir(), From: "haas <no-reply@example.com>"}
	if err := out.Send("ada@acme.com\r\nBcc: victim@example.org", "hello", "body"); err == nil {
		t.Error("an address may not carry a header")
	}
	if err := out.Send("ada@acme.com", "hello\nBcc: victim@example.org", "body"); err == nil {
		t.Error("a subject may not carry a header")
	}
	if files, _ := os.ReadDir(out.Dir); len(files) != 0 {
		t.Errorf("nothing should have been written: %d files", len(files))
	}
}

func TestTheOutboxKeepsWhatAPersonWouldRead(t *testing.T) {
	out := Outbox{Dir: t.TempDir(), From: "haas <no-reply@example.com>"}
	if err := out.Send("ada@acme.com", "Confirm your address", "Follow this link:\nhttps://x.test/verify?t=abc"); err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(out.Dir)
	if len(files) != 1 {
		t.Fatalf("%d files", len(files))
	}
	text, _ := os.ReadFile(filepath.Join(out.Dir, files[0].Name()))
	for _, want := range []string{"To: ada@acme.com", "Subject: Confirm your address", "Content-Type: text/plain; charset=utf-8", "https://x.test/verify?t=abc"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("the mail lacks %q:\n%s", want, text)
		}
	}
}

func TestMailSettingsAreReadOrRefusedByName(t *testing.T) {
	if m, err := MailFrom("", t.TempDir(), ""); err != nil || m == nil {
		t.Errorf("no setting means the outbox: %v", err)
	}
	m, err := MailFrom("smtp://user:p%40ss@mail.example.com:587?from=haas@example.com", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := m.(SMTP); got.Addr != "mail.example.com:587" || got.User != "user" || got.Password != "p@ss" || got.From != "haas@example.com" {
		t.Errorf("%+v", got)
	}
	for _, bad := range []string{"smtp://mail.example.com:587", "http://x", "smtp://", "nonsense"} {
		if _, err := MailFrom(bad, "", ""); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
}

// A server that speaks just enough SMTP to take one message, on the machine
// itself, where a password may go without TLS.
func TestSMTPDeliversTheMessageToTheServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	got := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		in := bufio.NewReader(conn)
		say := func(line string) { conn.Write([]byte(line + "\r\n")) }
		say("220 test ESMTP")
		var data strings.Builder
		reading := false
		for {
			line, err := in.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			switch {
			case reading && line == ".":
				reading = false
				say("250 queued")
				got <- data.String()
			case reading:
				data.WriteString(line + "\n")
			case strings.HasPrefix(line, "EHLO"):
				say("250-test")
				say("250 AUTH PLAIN")
			case strings.HasPrefix(line, "AUTH"):
				say("235 ok")
			case strings.HasPrefix(line, "DATA"):
				reading = true
				say("354 go")
			case strings.HasPrefix(line, "QUIT"):
				say("221 bye")
				return
			default:
				say("250 ok")
			}
		}
	}()
	send := SMTP{Addr: listener.Addr().String(), User: "u", Password: "p", From: "haas <no-reply@example.com>"}
	if err := send.Send("ada@acme.com", "Confirm", "the link"); err != nil {
		t.Fatal(err)
	}
	text := <-got
	if !strings.Contains(text, "To: ada@acme.com") || !strings.Contains(text, "the link") {
		t.Errorf("the server was sent:\n%s", text)
	}
}
