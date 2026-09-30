package cloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

// cloudflareAPI is where Cloudflare's Email Sending is reached.
const cloudflareAPI = "https://api.cloudflare.com/client/v4"

// Cloudflare sends through Cloudflare's Email Sending, which takes a message as a request
// with an API token and not over SMTP. The sending address must belong to a domain that
// has been onboarded to Email Sending.
type Cloudflare struct {
	Account, Token, From string
	// API is Cloudflare's address; a test points it at a server of its own.
	API string
	// Wait is how long to wait before asking again after a refusal that may pass.
	Wait time.Duration
}

const cloudflareTries = 3

func (c Cloudflare) Send(to, subject, body string) error {
	for _, field := range []string{to, subject} {
		if strings.ContainsAny(field, "\r\n") {
			return errors.New("a mail header may not have a line break in it")
		}
	}
	from, err := mail.ParseAddress(c.From)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"to":      to,
		"from":    map[string]string{"address": from.Address, "name": from.Name},
		"subject": subject,
		"text":    body,
	})
	if err != nil {
		return err
	}
	var last error
	for try := 0; try < cloudflareTries; try++ {
		if try > 0 {
			time.Sleep(c.Wait * time.Duration(1<<(try-1)))
		}
		var again bool
		if again, last = c.once(payload); last == nil || !again {
			return last
		}
	}
	return last
}

// once asks Cloudflare to send the message, and says whether asking again may work.
func (c Cloudflare) once(payload []byte) (retry bool, err error) {
	api := c.API
	if api == "" {
		api = cloudflareAPI
	}
	request, err := http.NewRequest("POST", api+"/accounts/"+c.Account+"/email/sending/send", bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 20 * time.Second}).Do(request)
	if err != nil {
		return true, err
	}
	defer response.Body.Close()
	text, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	var answer struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Result struct {
			Bounced []string `json:"permanent_bounces"`
		} `json:"result"`
	}
	_ = json.Unmarshal(text, &answer)
	switch {
	case response.StatusCode == http.StatusOK && answer.Success && len(answer.Result.Bounced) == 0:
		return false, nil
	case response.StatusCode == http.StatusOK && answer.Success:
		return false, errors.New("the address was refused by the mail server")
	}
	retry = response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
	var why []string
	for _, each := range answer.Errors {
		why = append(why, fmt.Sprintf("%d %s", each.Code, each.Message))
	}
	return retry, fmt.Errorf("cloudflare refused the mail: HTTP %d %s", response.StatusCode, strings.Join(why, "; "))
}
