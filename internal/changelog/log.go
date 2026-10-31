// Package changelog is the record of what changed on the platform: who signed
// in, who was refused, who changed a role or a setup, and when the settings that
// decide who may do what changed.
//
// It is not a task's record. A task's record says what an agent did. This says
// what the people who run the platform did to it, so a security reviewer can
// answer who changed it and when.
//
// The file is JSON lines, appended and never rewritten. Each line names the hash
// of the line before it, so an edited line, a removed one or an inserted one
// breaks the chain at that place, and each line is signed by the deployment's
// signing key when it holds one. What no chain can show is a tail cut off the
// end: a copy taken earlier, such as an export, is what shows that.
package changelog

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/signing"
)

// Kinds of event.
const (
	SignedIn         = "signin"
	Refused          = "signin.refused"
	SignedOut        = "signout"
	RoleChanged      = "role.changed"
	SetupSaved       = "setup.saved"
	SkillWritten     = "skill.written"
	SkillDropped     = "skill.dropped"
	ServerStarted    = "server.started"
	SettingsChanged  = "settings.changed"
	TokenCreated     = "token.created"
	TokenRevoked     = "token.revoked"
	HoldSet          = "retention.hold"
	HoldReleased     = "retention.release"
	Swept            = "retention.swept"
	MemberInvited    = "member.invited"
	MemberRole       = "member.role"
	MemberRemoved    = "member.removed"
	BudgetSet        = "budget.set"
	MemberBudgetSet  = "budget.member.set"
	ChatDefaultSet   = "chat.default"
	RunTokensSet     = "runs.tokens"
	SandboxConfirmed = "sandbox.confirmed"
	ProviderAdded    = "provider.added"
	ProviderRetired  = "provider.retired"
	KeyReplaced      = "provider.key"
	AddressAdded     = "provider.address.added"
	AddressRemoved   = "provider.address.removed"
	ModelAdded       = "model.added"
	ModelRetired     = "model.retired"
	PriceSet         = "model.price"
	PolicySet        = "model.policy"
	AccessSet        = "model.access"
	ProviderEnabled  = "provider.enabled"
	ProviderDisabled = "provider.disabled"
	ModelEnabled     = "model.enabled"
	ModelDisabled    = "model.disabled"
	RegistryRestored = "registry.restored"
	SetupEnabled     = "setup.enabled"
	SetupDisabled    = "setup.disabled"
	SkillRestored    = "skill.restored"

	IntegrationImported = "integration.imported"
	IntegrationEnabled  = "integration.enabled"
	IntegrationDisabled = "integration.disabled"
	IntegrationRestored = "integration.restored"
	IntegrationPrograms = "integration.programs"
	IntegrationScoped   = "integration.scoped"

	ConnectorEnabled      = "connector.enabled"
	ConnectorDisabled     = "connector.disabled"
	ConnectorConnected    = "connector.connected"
	ConnectorDisconnected = "connector.disconnected"
)

// FromFrontDoor says whether a kind of event is one that an organization's front
// door reports. Only these are taken from its inbox: what else is in the record was
// written by this server about itself.
func FromFrontDoor(kind string) bool {
	switch kind {
	case MemberInvited, MemberRole, MemberRemoved:
		return true
	}
	return false
}

// InboxKey is the field of an event's details that names the inbox file it came from,
// which is how an event that was taken twice is written once.
const InboxKey = "inbox"

// Event is one line of the log.
type Event struct {
	Seq  int64  `json:"seq"`
	At   string `json:"at"`
	Who  string `json:"who,omitempty"`
	Role string `json:"role,omitempty"`
	// From is where the request came from.
	From string            `json:"from,omitempty"`
	Kind string            `json:"kind"`
	What map[string]string `json:"what,omitempty"`
	// Prev is the hash of the line before, and empty for the first.
	Prev string `json:"prev"`
	// Hash covers every field above, in this order, as this package writes them.
	Hash string `json:"hash"`
	// Key, Scheme and Sig are the deployment's signature over Hash, when it holds
	// a key.
	Key    string `json:"key,omitempty"`
	Scheme string `json:"scheme,omitempty"`
	Sig    string `json:"sig,omitempty"`
}

// hashOf is the hash a line carries: the event as written, with no hash and no
// signature yet.
func hashOf(e Event) (string, error) {
	e.Hash, e.Key, e.Scheme, e.Sig = "", "", "", ""
	text, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(text)
	return hex.EncodeToString(sum[:]), nil
}

// BrokenError says where a log stopped holding together, and why.
type BrokenError struct {
	Line   int
	Reason string
}

func (e *BrokenError) Error() string {
	return fmt.Sprintf("the change log does not hold together at line %d: %s", e.Line, e.Reason)
}

// Report is what a whole log says about itself.
type Report struct {
	Events int
	// Head is the hash of the last line, which anybody holding a copy of it can
	// use to tell later that lines were cut off the end.
	Head string
	// Signed and Unsigned count the lines by whether they carry a signature.
	Signed   int
	Unsigned int
	// Signers counts the signed lines each key in the keyring stands behind, and
	// Unknown those whose key is not in it, by key id.
	Signers map[string]int
	Unknown map[string]int
}

// Verify reads a whole log and checks it line by line. A break is a
// *BrokenError. A line signed by a key the keyring does not hold is not a break:
// it is counted, so the verifier can say whose it is once the key is found.
func Verify(r io.Reader, keyring signing.Keyring) (Report, error) {
	report := Report{Signers: map[string]int{}, Unknown: map[string]int{}}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for line := 1; scanner.Scan(); line++ {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return report, &BrokenError{Line: line, Reason: "it is not an event"}
		}
		if e.Seq != int64(report.Events)+1 {
			return report, &BrokenError{Line: line, Reason: fmt.Sprintf("it is number %d and %d was next", e.Seq, report.Events+1)}
		}
		if e.Prev != report.Head {
			return report, &BrokenError{Line: line, Reason: "it does not follow the line before it"}
		}
		if want, err := hashOf(e); err != nil || e.Hash != want {
			return report, &BrokenError{Line: line, Reason: "its content does not match its hash"}
		}
		if e.Sig == "" {
			report.Unsigned++
		} else if err := checkSignature(e, keyring, &report); err != nil {
			return report, &BrokenError{Line: line, Reason: err.Error()}
		}
		report.Events++
		report.Head = e.Hash
	}
	if err := scanner.Err(); err != nil {
		return report, fmt.Errorf("the change log could not be read: %w", err)
	}
	return report, nil
}

func checkSignature(e Event, keyring signing.Keyring, report *Report) error {
	signature := signing.Signature{Version: signing.Version, Scheme: e.Scheme, Key: e.Key, Value: e.Sig}
	named, err := keyring.Signer(signature, []byte(e.Hash))
	var unknown signing.UnknownKeyError
	switch {
	case errors.As(err, &unknown):
		report.Unknown[unknown.ID]++
		report.Signed++
	case err != nil:
		return errors.New("its signature does not hold")
	default:
		report.Signers[named.Name]++
		report.Signed++
	}
	return nil
}

// Log is a change log open for appending.
type Log struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	signer signing.Signer
	seq    int64
	head   string
	roles  map[string]string
	start  map[string]string
	// taken is the inbox files whose events are in the log.
	taken map[string]bool
	now   func() time.Time
}

// Open reads the log at path, refusing one that does not hold together, and
// opens it for appending. A log that has been tampered with is not written to:
// what would follow it would look as sound as what came before. The signer may
// be nil, and then lines carry a hash and no signature.
func Open(path string, signer signing.Signer, keyring signing.Keyring) (*Log, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	l := &Log{path: path, signer: signer, roles: map[string]string{}, start: map[string]string{}, taken: map[string]bool{}, now: time.Now}
	if err := l.load(path, keyring); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	l.file = file
	return l, nil
}

// load verifies the log and learns what it needs to carry on: where the chain
// ends, each person's last role, and the settings the server last started with.
func (l *Log) load(path string, keyring signing.Keyring) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	report, err := Verify(file, keyring)
	if err != nil {
		return err
	}
	l.seq, l.head = int64(report.Events), report.Head
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for scanner.Scan() {
		var e Event
		if json.Unmarshal(scanner.Bytes(), &e) == nil {
			l.learn(e)
		}
	}
	return scanner.Err()
}

func (l *Log) learn(e Event) {
	switch e.Kind {
	case SignedIn:
		if e.Who != "" {
			l.roles[strings.ToLower(e.Who)] = e.Role
		}
	case ServerStarted:
		l.start = e.What
	}
	if id := e.What[InboxKey]; id != "" {
		l.taken[id] = true
	}
}

// Taken says whether the event of an inbox file is already in the log. A server that
// stopped between writing an event and removing its file finds it there.
func (l *Log) Taken(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.taken[id]
}

// Append writes one event, numbered, chained to the one before, and signed.
// It is safe for many goroutines. The event has been written and synced to
// disk when it returns.
func (l *Log) Append(e Event) (Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e.Seq = l.seq + 1
	if e.At == "" {
		e.At = l.now().UTC().Format(time.RFC3339Nano)
	}
	e.Prev = l.head
	hash, err := hashOf(e)
	if err != nil {
		return Event{}, err
	}
	e.Hash = hash
	if l.signer != nil {
		value, err := l.signer.Sign([]byte(hash))
		if err != nil {
			return Event{}, fmt.Errorf("the change log could not sign an event: %w", err)
		}
		e.Key, e.Scheme, e.Sig = l.signer.ID(), l.signer.Scheme(), base64.StdEncoding.EncodeToString(value)
	}
	text, err := json.Marshal(e)
	if err != nil {
		return Event{}, err
	}
	if _, err := l.file.Write(append(text, '\n')); err != nil {
		return Event{}, err
	}
	if err := l.file.Sync(); err != nil {
		return Event{}, err
	}
	l.seq, l.head = e.Seq, e.Hash
	l.learn(e)
	return e, nil
}

// LastRole is the role somebody last signed in with, and whether they ever had.
func (l *Log) LastRole(who string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	role, found := l.roles[strings.ToLower(who)]
	return role, found
}

// LastSettings is the settings the server recorded when it last started.
func (l *Log) LastSettings() map[string]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	copied := make(map[string]string, len(l.start))
	for key, value := range l.start {
		copied[key] = value
	}
	return copied
}

// Path is where the log is kept.
func (l *Log) Path() string { return l.path }

// Contains says whether a line with this hash is in the log, and its number. A
// head somebody kept earlier is found in the log until lines are cut off the end
// of it, and then it is not.
func Contains(r io.Reader, hash string) (int64, bool, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for scanner.Scan() {
		var e Event
		if json.Unmarshal(scanner.Bytes(), &e) == nil && e.Hash == hash {
			return e.Seq, true, nil
		}
	}
	return 0, false, scanner.Err()
}

// Close ends the log.
func (l *Log) Close() error { return l.file.Close() }
