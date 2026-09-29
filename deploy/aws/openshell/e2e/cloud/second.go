package main

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// secondTurns asks each agent, in the same conversation, for what it was told, without
// looking in any file: it is answered from the conversation the harness resumed, or from
// the file the first turn left in the instance, and either way from the same computer's turn.
func (r run) secondTurns(slug string, threads []*thread) {
	var group sync.WaitGroup
	for _, each := range threads {
		group.Add(1)
		go func() {
			defer group.Done()
			each.say(r, slug, "What is the secret word? Write it, and only it, into a file called answer.txt in the current directory.")
		}()
	}
	group.Wait()
	for _, each := range threads {
		jobs, err := r.host.finished(slug, each.person.email, 2, r.within)
		check(err == nil, "%s's second turn is done: %v", each.person.email, states(jobs))
		answer, err := r.host.fileIn(slug, each.instance, "answer.txt")
		check(err == nil && strings.Contains(answer, each.secret), "the second turn found what the first left: %q", answer)
	}
}

var computerName = regexp.MustCompile(`c-[0-9a-f]{8}-[0-9a-f]{8}`)

// observe checks what the host holds against who asked: one computer for each person, all
// of them the organisation's, and nothing else left in the gateway.
func (r run) observe(slug string, threads []*thread) {
	listed, err := r.host.computers()
	must(err, "haas cloud computers")
	fmt.Println(listed)
	held := map[string]bool{}
	for _, name := range computerName.FindAllString(listed, -1) {
		held[name] = true
	}
	check(len(held) == len(threads), "the host lists %d computers for %d people", len(held), len(threads))
	orgs := map[string]bool{}
	for name := range held {
		orgs[name[:10]] = true
	}
	check(len(orgs) == 1, "every one of them carries the organisation's mark")
	for _, each := range threads {
		check(strings.Contains(listed, each.person.email) && strings.Contains(listed, slug), "%s has a computer in %s", each.person.email, slug)
	}
	names, err := r.host.sandboxes()
	must(err, "the gateway's sandboxes")
	inGateway := 0
	for _, name := range names {
		if held[name] {
			inGateway++
		}
	}
	check(inGateway == len(held), "each listed computer is a sandbox on the gateway (%d of %d)", inGateway, len(held))
	check(len(names) == len(held), "nothing else is left on the gateway: %v", names)
}

// isolation makes a second organisation, and checks the first cannot be reached from it or
// by another of its own people.
func (r run) isolation(slug string, threads []*thread) {
	const other = "dan@globex-industries.com"
	second := r.signUp(other, "Globex Industries")
	dan := adminsByEmail[other]
	r.firstTurns(second, []*visitor{dan})

	bob, cy := threads[0], threads[1]
	seen, err := bob.person.get(r.orgAt(slug) + "/instances/" + cy.instance)
	must(err, "another person's instance")
	check(seen.status == 404, "%s cannot open %s's agent in their own organisation (%d)", bob.person.email, cy.person.email, seen.status)
	to, err := bob.person.where(r.orgAt(second) + "/")
	must(err, "another organisation")
	check(strings.Contains(to, "/login"), "%s is not signed in to %s: sent to %s", bob.person.email, second, to)

	listed, err := r.host.computers()
	must(err, "haas cloud computers")
	fmt.Println(listed)
	held := map[string]bool{}
	for _, name := range computerName.FindAllString(listed, -1) {
		held[name] = true
	}
	check(len(held) == len(threads)+1, "two organisations, %d computers", len(held))
	check(strings.Contains(listed, second) && strings.Contains(listed, other), "%s's computer is listed under %s", other, second)
}
