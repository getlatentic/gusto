package cloud

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// Mailer sends one plain message to one address.
type Mailer interface {
	Send(to, subject, body string) error
}

// message is the text of a mail. An address or a subject with a line break in
// it would add a header of the sender's choosing, so neither may have one.
func message(from, to, subject, body string) ([]byte, error) {
	for _, field := range []string{from, to, subject} {
		if strings.ContainsAny(field, "\r\n") {
			return nil, errors.New("a mail header may not have a line break in it")
		}
	}
	id := make([]byte, 12)
	rand.Read(id)
	var out strings.Builder
	fmt.Fprintf(&out, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s@haas>\r\n",
		from, to, subject, time.Now().UTC().Format(time.RFC1123Z), hex.EncodeToString(id))
	out.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n")
	out.WriteString(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(out.String()), nil
}

// outboxSequence numbers the mail in the order it is sent, which two sent in one
// millisecond are otherwise not told apart by.
var outboxSequence atomic.Uint64

// Outbox writes each mail to a file in a directory instead of sending it: for a
// machine with no mail server, and for a test that reads what a person would.
type Outbox struct {
	Dir  string
	From string
}

func (o Outbox) Send(to, subject, body string) error {
	text, err := message(o.From, to, subject, body)
	if err != nil {
		return err
	}
	id := make([]byte, 6)
	rand.Read(id)
	name := fmt.Sprintf("%s-%06d-%s.eml", time.Now().UTC().Format("20060102-150405.000"), outboxSequence.Add(1), hex.EncodeToString(id))
	return os.WriteFile(filepath.Join(o.Dir, name), text, 0o600)
}

// SMTP sends through a mail server. The connection is upgraded to TLS when the
// server offers it, and the password is sent only over one, or to the machine
// itself.
type SMTP struct {
	Addr, User, Password, From string
}

func (m SMTP) Send(to, subject, body string) error {
	text, err := message(m.From, to, subject, body)
	if err != nil {
		return err
	}
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(m.Addr)
	if err != nil {
		return err
	}
	var auth smtp.Auth
	if m.User != "" {
		auth = smtp.PlainAuth("", m.User, m.Password, host)
	}
	return smtp.SendMail(m.Addr, auth, from.Address, []string{to}, text)
}

// MailFrom reads HAAS_CLOUD_MAIL: `outbox`,
// smtp://user:password@host:587?from=name@example.com, or
// cloudflare://<account id>?from=name@example.com, whose API token is given apart from
// the setting so that it is kept where a secret is.
func MailFrom(setting, outbox, cloudflareToken string) (Mailer, error) {
	if setting == "" || setting == "outbox" {
		return Outbox{Dir: outbox, From: "haas <no-reply@localhost>"}, nil
	}
	const usage = "HAAS_CLOUD_MAIL is outbox, smtp://user:password@host:587?from=name@example.com, or cloudflare://<account id>?from=name@example.com"
	parsed, err := url.Parse(setting)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "smtp" && parsed.Scheme != "cloudflare") {
		return nil, errors.New(usage)
	}
	from := parsed.Query().Get("from")
	if _, err := mail.ParseAddress(from); err != nil {
		return nil, errors.New("HAAS_CLOUD_MAIL needs ?from=name@example.com, the address the mail is sent from")
	}
	if parsed.Scheme == "cloudflare" {
		if cloudflareToken == "" {
			return nil, errors.New("HAAS_CLOUD_MAIL_TOKEN is the Cloudflare API token that may send mail, and it is not set")
		}
		return Cloudflare{Account: parsed.Host, Token: cloudflareToken, From: from, Wait: time.Second}, nil
	}
	password, _ := parsed.User.Password()
	return SMTP{Addr: parsed.Host, User: parsed.User.Username(), Password: password, From: from}, nil
}
