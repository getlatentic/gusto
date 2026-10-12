package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/proxy"
)

// keyProvider is where a model key a person's shell may hold answers.
type keyProvider struct {
	Address string
	Auth    proxy.Auth
}

// keyProviders is the model keys haas knows by the name a provider's own tools read them from.
var keyProviders = map[string]keyProvider{
	"ANTHROPIC_API_KEY":  {"https://api.anthropic.com", proxy.Anthropic},
	"OPENAI_API_KEY":     {"https://api.openai.com", proxy.Bearer},
	"DEEPSEEK_API_KEY":   {"https://api.deepseek.com", proxy.Bearer},
	"OPENROUTER_API_KEY": {"https://openrouter.ai/api", proxy.Bearer},
}

// foundKeys is the names of the model keys this process's environment holds, and never what they are.
func foundKeys() []string {
	var found []string
	for name := range keyProviders {
		if os.Getenv(name) != "" {
			found = append(found, name)
		}
	}
	slices.Sort(found)
	return found
}

// useKey adds the key in one of the person's environment variables to their own haas, with the models they name, or the
// one they pick from what the provider lists. It signs in as the person, by a link it makes with their secret, and adds the
// key the way the chat page's form does, so the key goes to the computer's keychain and nowhere else.
func useKey(home string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: haas local use NAME [model...], where NAME is one of "+strings.Join(slices.Sorted(maps.Keys(keyProviders)), ", "))
		return 2
	}
	name, models := args[0], args[1:]
	provider, known := keyProviders[name]
	key := os.Getenv(name)
	switch {
	case !known:
		return fail(fmt.Errorf("haas does not know which provider %s is for: add it on the chat page instead", name))
	case key == "":
		return fail(fmt.Errorf("%s is not set in this shell", name))
	}
	setup, err := ensureLocal(home)
	if err != nil {
		return fail(err)
	}
	client, root, err := signedInClient(setup)
	if err != nil {
		return fail(err)
	}
	form := url.Values{"address": {provider.Address}, "auth": {string(provider.Auth)}, "key": {key}}
	if len(models) == 0 {
		if models, err = pickModel(client, root, form); err != nil {
			return fail(err)
		}
	}
	form.Set("models", strings.Join(models, ","))
	if err := postForm(client, root+"/setup/model", form); err != nil {
		return fail(err)
	}
	fmt.Printf("%s is in haas, with %s. Its key is kept on this computer.\n", name, strings.Join(models, ", "))
	return 0
}

// signedInClient is a client signed in to this person's haas, by a link of their own.
func signedInClient(setup localSetup) (*http.Client, string, error) {
	link, err := signInLink(setup, 5*time.Second)
	if err != nil {
		return nil, "", err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, "", err
	}
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	answer, err := client.Get(link)
	if err != nil {
		return nil, "", err
	}
	answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("haas did not sign this shell in: %s", answer.Status)
	}
	return client, setup.root(), nil
}

// pickModel asks the provider which models the key can use, through haas, and has the person pick one.
func pickModel(client *http.Client, root string, form url.Values) ([]string, error) {
	answer, err := postTo(client, root+"/setup/models", form)
	if err != nil {
		return nil, err
	}
	defer answer.Body.Close()
	var listed struct {
		Models []string `json:"models"`
		Error  string   `json:"error"`
	}
	if err := json.NewDecoder(answer.Body).Decode(&listed); err != nil {
		return nil, err
	}
	if listed.Error != "" {
		return nil, errors.New(listed.Error)
	}
	if len(listed.Models) == 0 {
		return nil, errors.New("the provider lists no model: name one after the key's name")
	}
	for i, model := range listed.Models {
		fmt.Printf("%3d  %s\n", i+1, model)
	}
	fmt.Print("Which model? Type its number: ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	choice, convErr := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || convErr != nil || choice < 1 || choice > len(listed.Models) {
		return nil, errors.New("no model was picked: name one after the key's name")
	}
	return []string{listed.Models[choice-1]}, nil
}

func postTo(client *http.Client, address string, form url.Values) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodPost, address, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", strings.TrimSuffix(strings.SplitN(address, "/setup", 2)[0], "/"))
	return client.Do(request)
}

// postForm sends the chat page's form, and reads haas's refusal off where it sends the person back to.
func postForm(client *http.Client, address string, form url.Values) error {
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer func() { client.CheckRedirect = nil }()
	answer, err := postTo(client, address, form)
	if err != nil {
		return err
	}
	answer.Body.Close()
	where, _ := url.Parse(answer.Header.Get("Location"))
	if refused := where.Query().Get("refused"); refused != "" {
		return errors.New(refused)
	}
	if answer.StatusCode != http.StatusSeeOther {
		return fmt.Errorf("haas answered %s", answer.Status)
	}
	return nil
}
