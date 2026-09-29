// Package cloud is the front door of a hosted haas: the accounts, the
// organisations they create, and the limits an organisation runs under. It runs
// no jobs. Each organisation's work is done by a haas server of its own.
package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// Refusals a sign-up can meet, each one a sentence a person can act on.
var (
	ErrBadEmail     = errors.New("that is not an email address")
	ErrFreeMail     = errors.New("sign up with the email address your company gave you")
	ErrDisposable   = errors.New("that email provider gives out addresses that expire, so it cannot be used to sign up")
	ErrBadOrgName   = errors.New("an organisation's name is 2 to 80 characters")
	ErrBadSlug      = errors.New("an organisation's address is 3 to 32 letters, digits and hyphens, and starts and ends with a letter or digit")
	ErrReservedSlug = errors.New("that address is kept, so pick another")
	ErrEmailTaken   = errors.New("that address already has an account, so sign in instead")
	ErrSlugTaken    = errors.New("that organisation address is taken, so pick another")
	ErrDomainFull   = errors.New("this company already has the most organisations it may make, so ask an admin there to invite you")
	// ErrNeedName is a sign-up with no name for an organisation, from an address
	// that is not a company's, so its name cannot come from the domain.
	ErrNeedName = errors.New("that address is not a company's, so give your organisation a name")
	// ErrSameCompany is a sign-up at a domain that has an organisation already,
	// with no name for this one: a company can have several, a subsidiary or a
	// team, and each has a name of its own.
	ErrSameCompany = errors.New("this company already has an organisation here, so name this one, a subsidiary or a team, or ask its admin to invite you")
)

// freeMail is the providers a person has for themselves and not for a company.
// A sign-up that is meant to start a company's organisation is not made from one
// unless the operator says so, which a demonstration does.
var freeMail = setOf("gmail.com", "googlemail.com", "outlook.com", "hotmail.com", "live.com", "msn.com",
	"yahoo.com", "ymail.com", "icloud.com", "me.com", "mac.com", "aol.com", "proton.me", "protonmail.com",
	"gmx.com", "gmx.net", "mail.com", "zoho.com", "yandex.com")

// disposable is providers whose addresses last minutes, which is what someone
// making many organisations to get many trials would use.
var disposable = setOf("mailinator.com", "guerrillamail.com", "10minutemail.com", "tempmail.com",
	"temp-mail.org", "yopmail.com", "sharklasers.com", "trashmail.com", "throwawaymail.com",
	"getnada.com", "dispostable.com", "maildrop.cc")

func setOf(members ...string) map[string]bool {
	set := make(map[string]bool, len(members))
	for _, member := range members {
		set[member] = true
	}
	return set
}

// NormalEmail is an address as the front door keeps it: lower case, with no
// name, no comment and no space in it. Two spellings of one address are one
// person.
func NormalEmail(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 254 || strings.ContainsAny(text, "<>\" \t\r\n,;") {
		return "", ErrBadEmail
	}
	parsed, err := mail.ParseAddress(text)
	if err != nil || parsed.Address != text {
		return "", ErrBadEmail
	}
	local, domain, found := strings.Cut(strings.ToLower(text), "@")
	if !found || local == "" || len(local) > 64 || !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", ErrBadEmail
	}
	// A domain is kept in its ASCII form, so a lookalike written with letters from
	// another alphabet is the different domain it is, and not a copy of a company's.
	ascii, err := idna.Lookup.ToASCII(domain)
	if err != nil || ascii == "" {
		return "", ErrBadEmail
	}
	return local + "@" + ascii, nil
}

// DomainOf is what follows the @ of a normal address.
func DomainOf(email string) string {
	_, domain, _ := strings.Cut(email, "@")
	return domain
}

// workEmail refuses an address a company would not have given out.
func workEmail(email string, allowFree bool, refused map[string]bool) error {
	domain := DomainOf(email)
	switch {
	case disposable[domain] || refused[domain]:
		return ErrDisposable
	case freeMail[domain] && !allowFree:
		return ErrFreeMail
	}
	return nil
}

// OrgName is a name as it is shown: spaces collapsed, and nothing in it that
// draws as something other than text.
func OrgName(text string) (string, error) {
	name := strings.Join(strings.Fields(text), " ")
	if n := len([]rune(name)); n < 2 || n > 80 {
		return "", ErrBadOrgName
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '<' || r == '>' {
			return "", ErrBadOrgName
		}
	}
	return name, nil
}

var slugShape = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,30}[a-z0-9]$`)

// reserved is addresses that would read as the service itself, or that a name
// server or a mail server takes for its own.
var reserved = setOf("www", "api", "app", "admin", "cloud", "static", "mail", "smtp", "imap", "ftp", "ns1", "ns2",
	"status", "docs", "help", "support", "billing", "login", "signin", "signup", "auth", "console", "dashboard",
	"root", "haas", "latentic", "getlatentic", "staging", "demo", "test", "localhost", "security", "abuse", "postmaster")

// SlugFor is the address a name suggests: its letters and digits, joined by
// hyphens. It may be too short to use, and then a person picks one.
func SlugFor(name string) string {
	slug := slugify(name)
	if len(slug) > 32 {
		slug = strings.Trim(slug[:32], "-")
	}
	return slug
}

// slugify is a name's letters and digits, lower case, joined by single hyphens.
func slugify(name string) string {
	var out strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			hyphen = false
		case !hyphen && out.Len() > 0:
			out.WriteByte('-')
			hyphen = true
		}
	}
	return strings.Trim(out.String(), "-")
}

// ValidSlug says whether an address can name an organisation.
func ValidSlug(slug string) error {
	switch {
	case !slugShape.MatchString(slug) || strings.Contains(slug, "--"):
		return ErrBadSlug
	case reserved[slug]:
		return ErrReservedSlug
	}
	return nil
}

// company is the part of a domain that names the company: acme in acme.com, in
// mail.acme.co.uk and in acme.io, found with the public suffix list so that a
// second-level suffix such as co.uk is not taken for the company.
func company(domain string) string {
	registrable, err := publicsuffix.EffectiveTLDPlusOne(domain)
	if err != nil {
		registrable = domain
	}
	name, _, _ := strings.Cut(registrable, ".")
	return name
}

// FromDomain is the name an organisation is given when its admin gave none, and
// the address that follows from it: acme-research.com is "Acme Research" at
// acme-research. An address at a free mail provider names nobody's company, so
// the admin is asked.
func FromDomain(domain string) (name string, err error) {
	if freeMail[domain] || disposable[domain] {
		return "", ErrNeedName
	}
	label := company(domain)
	// A company written in another alphabet has a label that is not a name to show.
	if strings.HasPrefix(label, "xn--") {
		return "", ErrNeedName
	}
	words := strings.Split(label, "-")
	for i, word := range words {
		if r := []rune(word); len(r) > 0 {
			words[i] = string(unicode.ToUpper(r[0])) + string(r[1:])
		}
	}
	return strings.Join(strings.Fields(strings.Join(words, " ")), " "), nil
}

// AddressFor is the address a company's domain gives its organisation: the whole
// of its registrable domain, hyphenated, so acme-research.com is acme-research-com
// and acme-research.co is acme-research-co. A domain is one company's alone, so
// nobody is put ahead of another by arriving first, and nobody is asked to take
// a worse address. One too long for an address is cut and given a digest of the
// whole, so two long domains that begin alike stay two.
func AddressFor(registrable string) string {
	slug := slugify(strings.ReplaceAll(registrable, ".", "-"))
	if len(slug) <= 32 {
		return slug
	}
	sum := sha256.Sum256([]byte(registrable))
	return strings.Trim(slug[:23], "-") + "-" + hex.EncodeToString(sum[:])[:8]
}
