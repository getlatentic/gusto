// Drives the public cloud as people would, on the machine that runs it: an admin signs up
// and confirms their address, invites three people, each opens the organization and asks
// an agent for something, and what the host holds afterwards is checked against what was
// asked. A second organization is made to see that the first cannot be reached from it. With -sandbox container the agents
// run in Docker containers, and the network is probed from inside one while it works.
package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var failures int

func check(ok bool, format string, args ...any) {
	if ok {
		fmt.Printf("ok    %s\n", fmt.Sprintf(format, args...))
		return
	}
	failures++
	fmt.Printf("FAIL  %s\n", fmt.Sprintf(format, args...))
}

func must(err error, what string) {
	if err != nil {
		fmt.Printf("FAIL  %s: %v\n", what, err)
		os.Exit(1)
	}
}

type run struct {
	host   host
	door   string
	domain string
	port   string
	model  string
	within time.Duration
}

func (r run) front() string            { return "http://" + r.domain + ":" + r.port }
func (r run) orgAt(slug string) string { return "http://" + slug + "." + r.domain + ":" + r.port }

func main() {
	var r run
	var team string
	flag.StringVar(&r.host.root, "root", "cloud", "the front door's GUSTO_CLOUD_ROOT")
	flag.StringVar(&r.host.gusto, "gusto", "gusto", "the gusto program")
	flag.StringVar(&r.host.gateway, "gateway", "spike", "the OpenShell gateway")
	flag.StringVar(&r.host.sandbox, "sandbox", "openshell", "what the agents run in: openshell or container")
	flag.StringVar(&r.host.probe, "probe", "", "with -sandbox container, the isolation check run inside the first agent's container")
	flag.StringVar(&r.door, "door", "127.0.0.1:8050", "where the front door listens")
	flag.StringVar(&r.domain, "domain", "gusto.test", "GUSTO_CLOUD_DOMAIN")
	flag.StringVar(&r.model, "model", "", "the model the agents are given")
	flag.StringVar(&team, "team", "ada@acme-research.com,bob@acme-research.com,cy@acme-research.com,di@acme-research.com", "the admin, then the people they invite")
	flag.DurationVar(&r.within, "within", 12*time.Minute, "how long a turn may take")
	flag.Parse()
	_, r.port, _ = strings.Cut(r.door, ":")

	emails := strings.Split(team, ",")
	admin, invited := emails[0], emails[1:]
	slug := r.signUp(admin, "Acme Research")
	people := r.onboard(slug, admin, invited)
	probed := r.host.probeFirstContainer(r.within)
	threads := r.firstTurns(slug, people)
	if probed != nil {
		result := <-probed
		fmt.Println(result.output)
		check(result.err == nil, "from inside a working agent's container only the model proxy is reachable: %v", result.err)
	}
	r.secondTurns(slug, threads)
	r.observe(slug, threads)
	r.isolation(slug, threads)
	if failures > 0 {
		fmt.Printf("\n%d checks failed\n", failures)
		os.Exit(1)
	}
	fmt.Println("\nevery check held")
}

// signUp makes the organization and confirms the admin's address from the mail they were sent.
func (r run) signUp(email, name string) string {
	admin := newVisitor(email, r.door)
	shown, err := admin.post(r.front()+"/signup", url.Values{"email": {email}, "org": {name}})
	must(err, "sign-up")
	slug := regexp.MustCompile(`([a-z0-9-]+)\.` + regexp.QuoteMeta(r.domain)).FindStringSubmatch(shown.body)
	check(shown.status == 200 && slug != nil, "%s signs up and is shown the organization's address (%s)", email, first(slug))
	if slug == nil {
		os.Exit(1)
	}
	token, err := linkFor(r.host.root+"/outbox", email, "/verify")
	must(err, "the confirmation mail")
	confirmed, err := admin.post(r.front()+"/verify", url.Values{"t": {token}})
	must(err, "confirming the address")
	check(confirmed.status == 200 && !strings.Contains(confirmed.body, "Confirm your email"), "the address is confirmed from the mailed link")
	r.waitReady(admin, slug[1])
	adminsByEmail[email] = admin
	return slug[1]
}

var adminsByEmail = map[string]*visitor{}

func first(found []string) string {
	if len(found) > 1 {
		return found[1]
	}
	return ""
}

// waitReady waits for the organization's own server, which the sign-up started.
func (r run) waitReady(v *visitor, slug string) {
	started := time.Now()
	for time.Since(started) < 3*time.Minute {
		to, err := v.where(r.front() + "/open")
		if err == nil && strings.HasPrefix(to, r.orgAt(slug)) {
			check(true, "%s's own server is up %.0f s after sign-up", slug, time.Since(started).Seconds())
			return
		}
		time.Sleep(time.Second)
	}
	check(false, "%s's own server did not come up", slug)
	os.Exit(1)
}

// onboard has the admin invite each person, and each accept from the mail they were sent.
func (r run) onboard(slug, admin string, invited []string) []*visitor {
	head := adminsByEmail[admin]
	var people []*visitor
	for _, email := range invited {
		sent, err := head.post(r.front()+"/org/invite", url.Values{"email": {email}})
		must(err, "inviting "+email)
		check(sent.status == 200, "%s is invited by %s", email, admin)
		token, err := linkFor(r.host.root+"/outbox", email, "/invite")
		must(err, "the invitation mail")
		person := newVisitor(email, r.door)
		joined, err := person.post(r.front()+"/invite", url.Values{"t": {token}})
		must(err, "accepting")
		check(joined.status == 200, "%s joins from the mailed link", email)
		people = append(people, person)
	}
	return people
}

// thread is a conversation a person started on an agent of their own.
type thread struct {
	person   *visitor
	instance string
	path     string
	secret   string
}

// firstTurns has every person open the organization, start an agent and ask it for
// something, all at once.
func (r run) firstTurns(slug string, people []*visitor) []*thread {
	threads := make([]*thread, len(people))
	var group sync.WaitGroup
	for i, person := range people {
		group.Add(1)
		go func() {
			defer group.Done()
			opened, err := person.get(r.front() + "/open")
			must(err, "opening the organization")
			check(opened.status == 200 && strings.HasPrefix(opened.url, r.orgAt(slug)), "%s opens the organization and is signed in there", person.email)
			started, err := person.post(r.orgAt(slug)+"/instances", url.Values{"harness": {"opencode"}, "model": {r.model}})
			must(err, "starting an agent")
			path := regexp.MustCompile(`/instances/([^/]+)/threads/[^/?#]+`).FindString(started.url)
			check(path != "", "%s starts an agent and is talking to it (%s)", person.email, path)
			if path == "" {
				return
			}
			secret := fmt.Sprintf("%s-%d", strings.ToUpper(strings.Split(person.email, "@")[0]), 1000+i)
			threads[i] = &thread{person: person, instance: strings.Split(path, "/")[2], path: path, secret: secret}
			threads[i].say(r, slug, "The secret word is "+secret+". Write it, and only it, into a file called secret.txt in the current directory.")
		}()
	}
	group.Wait()
	for _, each := range threads {
		if each == nil {
			os.Exit(1)
		}
		jobs, err := r.host.finished(slug, each.person.email, 1, r.within)
		check(err == nil, "%s's first turn is done: %v", each.person.email, states(jobs))
		word, err := r.host.fileIn(slug, each.instance, "secret.txt")
		check(err == nil && word == each.secret, "the file the agent wrote is in %s's instance: %q", each.person.email, word)
	}
	return threads
}

func (t *thread) say(r run, slug, said string) {
	sent, err := t.person.post(r.orgAt(slug)+t.path+"/say", url.Values{"said": {said}})
	must(err, "saying something")
	check(sent.status == 200, "%s asks their agent for something", t.person.email)
}

func states(jobs []jobOf) string {
	var seen []string
	for _, each := range jobs {
		seen = append(seen, fmt.Sprintf("%s exit %d", each.State, each.Exit))
	}
	return strings.Join(seen, ", ")
}
