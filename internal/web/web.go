// Package web is the pages a person uses: start a harness, run work on it,
// watch it happen, and read what every run left behind.
//
// Server-rendered, with the templates compiled into the binary, because a
// customer installs one artifact and a second origin with its own build step is
// not one artifact.
package web

import (
	"cmp"
	"context"
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlatentic/gusto/internal/apitoken"
	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/compare"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/integration"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/notify"
	"github.com/getlatentic/gusto/internal/orgsettings"
	"github.com/getlatentic/gusto/internal/preset"
	"github.com/getlatentic/gusto/internal/registry"
	"github.com/getlatentic/gusto/internal/roomlive"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/spend"
	"github.com/getlatentic/gusto/pkg/wire"
)

//go:embed templates/*.html
var pages embed.FS

type Server struct {
	index     *jobs.Index
	instances *instance.Store
	queue     *jobs.Queue
	// routineNews is told how each run of a routine ended, and where the server says its own pages are.
	routineNews notify.Every
	publicURL   string
	keeper      *keeper
	templates   *template.Template
	identity    *Identity
	catalog     []catalog.Entry
	// liveCatalog, when set, is asked for the catalog each time a page wants it, as on a person's own computer, where they
	// install a harness while the server runs.
	liveCatalog func() []catalog.Entry
	// foundKeys is the names of the model keys a person's shell had when gusto was installed on their computer.
	foundKeys []string
	// needs says what a person's computer lacks that gusto needs.
	needs func() []string
	// wall is which wall agents run behind on a person's own computer.
	wall   *Wall
	models Models
	// live is what the registry makes of the models now, when there is one.
	live func() Models
	// appFirstByte is how long a young app has to begin to answer, which a test shortens.
	appFirstByte time.Duration
	registry     *registry.Registry
	// apps and runner are the web apps people share from their agents' computers.
	apps       *apps.Store
	runner     AppRunner
	publishing sync.WaitGroup
	ledger     *spend.Ledger
	usage      usageIndex
	listing    taskIndex
	keyring    signing.Keyring
	// front is set when this server is one organization's in a hosted gusto, and
	// then rosterPath is the list of its people that the front door writes, and
	// inbox is where the front door leaves what its admins did, for the change
	// record.
	front      *frontDoor
	rosterPath string
	inbox      string
	taking     sync.Mutex
	admins     map[string]bool
	// orgSettings is what the organization's admins choose for it, and computers is
	// the record of the computers its people have.
	orgSettings *orgsettings.File
	// local is how the one person a server on their own computer runs for signs in, or nil.
	local *localSignIn
	// operatorDefault is what a new chat runs on when neither the person nor the organization has chosen.
	operatorDefault chatDefault
	computers       *computer.Registry
	// changes is the record of what changed on the platform, and turnedAway
	// counts the sign-ins written to it.
	changes *changelog.Log
	tokens  *apitoken.Store
	devices deviceCodes
	// keyed serializes the sends that carry a request id, so that two arriving together are one.
	keyed sync.Mutex
	// known remembers the hash of files in agents' folders; locks, with writers, make one writer at a time of each folder.
	known hashes
	locks map[string]*sync.Mutex
	// roomHold keeps a room's merges out while a person changes its folder; see UseRoomHold.
	roomHold  func(ctx context.Context, folder string, paths []string) (func(changed bool), error)
	writers   sync.Mutex
	retention *retained
	// sockets is what keeps a room's page up to date, and announce is what turns a job's changes into its room's events.
	sockets  *roomlive.Server
	announce *announcer
	health   Health
	wrong    attempts
	// localRepos are the directories a repository may be cloned from on this
	// server's own disk, which is none unless the operator names them.
	localRepos []string
	startedAt  time.Time
	disk       diskUse
	turnedAway refusals
	lineups    *compare.Board
	setups     *setup.Store
	presets    *preset.Store
	crew       *crew.Store
	// reaches says whether an agent calling in a shape can use a model, and whether natively; nil offers every model to every agent.
	reaches   func(model string, shape wire.Shape) (native, ok bool)
	installer agentInstaller
	replies   replyNotes
	servers   setup.Servers
	// integrations is the store of imported plugins, and cloner fetches one from a git address.
	integrations *integration.Store
	// desktop is the folder of the desktop app's releases, which the server hands out so that an app updates from the server it is connected to.
	desktop string
	// tickets are sign-ins a browser has made for the desktop app and the app has not yet taken.
	tickets tickets
	cloner  integration.Clone
	// inBuilding is whether a model inside the deployment's own network is
	// reachable, without which restricted work cannot start at all.
	inBuilding atomic.Bool
}

func New(index *jobs.Index, instances *instance.Store, queue *jobs.Queue, password string, identity *Identity, offered []catalog.Entry, models Models) (*Server, error) {
	keeper, err := newKeeper(password)
	if err != nil {
		return nil, err
	}
	s := &Server{index: index, instances: instances, queue: queue, keeper: keeper, identity: identity, catalog: offered, models: models}
	s.templates, err = template.New("").Funcs(helpers).Funcs(template.FuncMap{
		"waiting": s.waiting,
		"askedBy": askerOf,
		// Where a host that has a lease is extended, which is at the front door.
		"leaseExtend": s.leaseExtend,
		"appLink":     s.appLink,
		"costOf":      s.costOf,
		"replyIn":     s.replyIn,
		"installable": s.installable,
	}).ParseFS(pages, "templates/*.html")
	if err != nil {
		return nil, err
	}
	// A server made only to read who is who has no jobs, so nothing to hand on.
	if index != nil {
		index.OnFinish(s.handOff)
	}
	return s, nil
}

// UseSessionFile keeps what signs people in in a file, so that a restart does not sign everybody out.
func (s *Server) UseSessionFile(path string) error { return s.keeper.keep(path) }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	if s.front != nil {
		mux.HandleFunc("GET /login", s.toFrontDoor)
		mux.HandleFunc("GET /auth/entry", s.enter)
	} else {
		mux.HandleFunc("GET /login", s.loginPage)
		mux.HandleFunc("POST /login", s.signIn)
		if s.local != nil {
			mux.HandleFunc("GET /local/enter", s.enterLocal)
		}
		mux.HandleFunc("GET /auth/start", s.start)
		mux.HandleFunc("GET /auth/callback", s.back)
	}
	mux.HandleFunc("GET /auth/desktop/start", s.desktopStart)
	mux.HandleFunc("GET /auth/desktop/handoff", s.guard(s.desktopHandoff))
	mux.HandleFunc("GET /auth/desktop/redeem", s.desktopRedeem)
	mux.HandleFunc("POST /logout", s.signOut)
	mux.Handle("GET /static/", serveStatic())
	mux.HandleFunc("GET /keys/{file}", s.key)
	s.desktopRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })

	mux.HandleFunc("GET /{$}", s.guard(s.home))
	mux.HandleFunc("GET /files", s.guard(s.filesPage))
	mux.HandleFunc("GET /chat/folders", s.guard(s.chatFolders))
	mux.HandleFunc("POST /chat/folders", s.guard(s.newChatFolder))
	mux.HandleFunc("GET /chat", s.guard(s.chatPage))
	mux.HandleFunc("POST /chat", s.guard(s.startChat))
	mux.HandleFunc("POST /presets", s.guard(s.savePreset))
	mux.HandleFunc("POST /presets/{id}/delete", s.guard(s.deletePreset))
	mux.HandleFunc("GET /presets", s.guard(s.presetsPage))
	mux.HandleFunc("GET /new", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/presets", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /new/blank", s.guard(s.blankPage))
	mux.HandleFunc("GET /engines", s.guard(s.enginesPage))
	mux.HandleFunc("GET /compare", s.guard(s.comparePage))
	mux.HandleFunc("POST /compare", s.guard(s.startComparison))
	mux.HandleFunc("GET /compare/{id}", s.guard(s.lineupPage))
	mux.HandleFunc("GET /setups", s.guard(s.setupsPage))
	mux.HandleFunc("GET /instances/new", s.guard(s.setUp))
	mux.HandleFunc("POST /instances", s.guard(s.startInstance))
	mux.HandleFunc("GET /instances/{id}", s.guard(s.instancePage))
	mux.HandleFunc("POST /instances/{id}/remove", s.guard(s.removeInstance))
	mux.HandleFunc("GET /instances/{id}/file", s.guard(s.folderFile))
	mux.HandleFunc("GET /instances/{id}/files", s.guard(s.folderPage))
	mux.HandleFunc("GET /instances/{id}/raw", s.guard(s.folderRaw))
	mux.HandleFunc("POST /instances/{id}/tasks", s.guard(s.runTask))
	mux.HandleFunc("POST /instances/{id}/model", s.guard(s.changeModel))
	mux.HandleFunc("POST /instances/{id}/apps", s.guard(s.publishApp))
	mux.HandleFunc("POST /instances/{id}/apps/{app}/remove", s.guard(s.removeApp))
	mux.HandleFunc("/apps/{id}/{rest...}", s.guard(s.app))
	mux.HandleFunc("/apps/{id}", s.guard(s.app))
	mux.HandleFunc("POST /instances/{id}/routines", s.guard(s.addRoutine))
	mux.HandleFunc("POST /instances/{id}/routines/{routine}/{action}", s.guard(s.changeRoutine))
	mux.HandleFunc("POST /instances/{id}/threads", s.guard(s.openThread))
	mux.HandleFunc("GET /instances/{id}/threads/{thread}", s.guard(s.conversation))
	mux.HandleFunc("GET /instances/{id}/threads/{thread}/turns", s.guard(s.turns))
	mux.HandleFunc("GET /instances/{id}/threads/{thread}/live", s.guard(s.turnLive))
	mux.HandleFunc("POST /instances/{id}/threads/{thread}/say", s.guard(s.say))
	mux.HandleFunc("GET /templates/{name}", s.guard(s.templatePage))
	mux.HandleFunc("POST /templates/{name}", s.guard(s.startTemplate))
	mux.HandleFunc("GET /setups/{name}", s.guard(s.setupPage))
	s.roomRoutes(mux)
	if s.sockets != nil {
		mux.Handle("GET "+livePath, s.sockets.Handler())
	}
	s.memberRoutes(mux)
	mux.HandleFunc("GET /jobs", s.guard(s.list))
	s.mountProtocol(mux)
	mux.HandleFunc("GET /device", s.guard(s.devicePage))
	mux.HandleFunc("POST /device", s.guard(s.approveDevice))
	mux.HandleFunc("POST /device/{id}/revoke", s.guard(s.endDeviceToken))
	mux.HandleFunc("GET /jobs/{id}", s.guard(s.job))
	mux.HandleFunc("POST /jobs/{id}/stop", s.guard(s.stopPage))
	mux.HandleFunc("GET /jobs/{id}/files/{name}", s.guard(s.file))
	mux.HandleFunc("GET /jobs/{id}/bundle", s.guard(s.bundle))
	mux.HandleFunc("GET /watch/{id}", s.guard(s.watch))
	mux.HandleFunc("GET /spend", s.guard(s.spendPage))
	s.mountUsage(mux)
	s.consoleRoutes(mux)
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("GET /metrics", s.sent(apitoken.Metrics, s.metrics))
	// Conversations were held at /talk before an instance could hold more than
	// one. A link somebody kept still finds its instance.
	mux.HandleFunc("GET /talk", moved("/"))
	mux.HandleFunc("GET /talk/{id}", moved("/instances/{id}"))
	return s.secure(s.byApp(mux))
}

// moved redirects an address that used to mean something here to where it
// means it now, filling {id} from the request.
func moved(to string) http.HandlerFunc { return redirected(to, http.StatusMovedPermanently) }

// redirected sends a request to another address with the status given, filling
// the names in {braces} from the request's path.
func redirected(to string, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		escaped := func(name string) string { return url.PathEscape(r.PathValue(name)) }
		filled := strings.NewReplacer("{id}", escaped("id"), "{name}", escaped("name"), "{skill}", escaped("skill")).Replace(to)
		http.Redirect(w, r, filled, status)
	}
}

// productName is what every page calls itself.
const productName = "gusto"

func (s *Server) render(w http.ResponseWriter, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// An inline script runs only with the nonce this response's policy names.
	if fields, ok := data.(map[string]any); ok {
		marks := markOf(w)
		fields["Nonce"] = nonceOf(w)
		fields["Admin"] = marks.admin
		if s.front != nil && s.front.Org != "" {
			fields["Org"] = s.front.Org
			fields["OrgName"] = cmp.Or(s.front.Name, s.front.Org)
			fields["Organizations"] = s.front.URL + "/orgs"
			fields["OrgsFrom"] = s.front.URL + "/orgs.json"
		}
		// Every page of the app carries the side list drawn with it, so that moving between pages does not redraw it.
		if _, carried := fields["Rail"]; !carried && fields["Console"] == nil && marks.req != nil && s.keeper.knows(marks.req) {
			fields["Rail"] = s.railFor(marks.req, "")
		}
	}
	if err := s.templates.ExecuteTemplate(w, page, data); err != nil {
		slog.Error("a page could not be drawn", "page", page, "error", err)
		http.Error(w, "the page could not be drawn", http.StatusInternalServerError)
	}
}
