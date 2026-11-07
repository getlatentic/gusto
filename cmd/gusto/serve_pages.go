package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/agentinstall"
	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/notify"
	"github.com/getlatentic/gusto/internal/orgsettings"
	"github.com/getlatentic/gusto/internal/retention"
	"github.com/getlatentic/gusto/internal/roomlive"
	"github.com/getlatentic/gusto/internal/web"
	"github.com/getlatentic/gusto/pkg/wire"
)

// openPages makes the pages, with the agents the boxes offer and the way people sign in. The issuer is reached here, so a wrong
// one fails at the start rather than at somebody's first sign-in.
func (s *server) openPages() error {
	offered := s.boxes.Offered()
	for _, entry := range offered {
		if !entry.Ready {
			fmt.Printf("  %-10s not offered: %s\n", entry.Name, entry.Why)
		}
	}
	access := s.cfg.access
	identity, err := web.NewIdentity(context.Background(), access.issuer, access.clientID, access.clientSecret, access.redirect,
		web.Access{
			Allowed:      access.allowed,
			RunnerGroups: access.runnerGroups,
			AdminGroups:  access.adminGroups,
			GroupsClaim:  access.groupsClaim,
			Scopes:       access.scopes,
		})
	if err != nil {
		return err
	}
	if identity.Configured() {
		fmt.Println("signing in with", access.issuer)
	}
	if s.pages, err = web.New(s.index, s.instances, s.queue, s.cfg.core.password, identity, offered, web.Models(s.cfg.way.models)); err != nil {
		return err
	}
	s.pages.UseRoomHold(s.rooms.hub.Hold)
	if wall != nil {
		s.pages.UseWall(theWall())
	}
	return nil
}

// offerThisComputer is what a server for one person on their own computer has: the keys found there, what is still to install,
// a folder gusto installs agents it does not find into, and sign-in as the person. Every server reads the agents its boxes offer
// as each page asks, and an agent run here is found on the PATH a person may have just changed.
func (s *server) offerThisComputer() error {
	local := s.cfg.core.local
	here := local && s.cfg.boxes.here()
	if local {
		s.pages.UseFoundKeys(s.cfg.access.foundKeys)
		s.pages.UseNeeds(func() []string {
			adoptRecordedPath()
			return localNeeds()
		})
		s.pages.UseLocalSignIn(s.cfg.core.password, localPerson())
	}
	if here {
		dir, err := filepath.Abs(s.cfg.served.agents)
		if err != nil {
			return err
		}
		s.installer = &agentinstall.Installer{Dir: dir}
		os.Setenv("PATH", s.installer.WithBin(os.Getenv("PATH")))
		s.pages.UseInstaller(s.installer)
	}
	s.pages.UseLiveCatalog(func() []catalog.Entry {
		if here {
			adoptRecordedPath()
			os.Setenv("PATH", s.installer.WithBin(os.Getenv("PATH")))
		}
		return s.boxes.Offered()
	})
	return nil
}

// shareApps shares the apps agents built, run in their owners' computers when the boxes have them, and the connectors a turn's
// tools sign in with.
func (s *server) shareApps() error {
	if host, ok := s.boxes.(appHost); ok && host.Apps() != nil {
		s.pages.UseApps(s.shared, host.Apps())
		s.pages.ReconcileApps()
	} else {
		s.pages.UseApps(s.shared, nil)
	}
	if s.connect != nil {
		s.pages.UseConnectors(s.connect)
	}
	return nil
}

// applyOrgSettings is what an organization's admins chose: each member's budget and whether runs get plain tokens. Settings that
// cannot be read hold nobody to a budget, since the organization's caps and the credit still hold, and leave runs bound, the
// safer way.
func (s *server) applyOrgSettings() error {
	path := s.cfg.served.orgSettings
	if path == "" {
		return nil
	}
	s.pages.UseOrgSettings(path)
	settings := orgsettings.Open(path)
	s.way.UseMemberBudgets(func(person string) (float64, bool) {
		chosen, err := settings.Read()
		if err != nil {
			return 0, false
		}
		return chosen.MemberBudget(person)
	})
	s.way.UseRunTokens(func() bool {
		chosen, err := settings.Read()
		return err == nil && chosen.PlainRunTokens
	})
	return nil
}

// offerModels is what people think with: the chat's default, the organization's registry of models, what was spent, whether a
// model answers, and the settings that change as the server runs.
func (s *server) offerModels() error {
	if agent, model, found := strings.Cut(s.cfg.news.chatDefault, "="); found {
		s.pages.UseChatDefault(strings.TrimSpace(agent), strings.TrimSpace(model))
	}
	reg, offeredNow, err := openRegistry(s.way, s.cfg.access.cloudURL != "", s.cfg.core.local, func() { s.pages.UseInBuilding(s.way.InBuilding()) })
	if err != nil {
		return err
	}
	if reg != nil {
		s.pages.UseRegistry(reg, offeredNow)
	}
	s.pages.UseLedger(s.way.ledger)
	if s.way.proxy != nil {
		s.pages.UseReach(func(model string, shape wire.Shape) (bool, bool) {
			return s.way.proxy.Reaches(context.Background(), model, shape)
		})
	}
	liveCtx, stopLive := context.WithCancel(context.Background())
	s.started(stopLive)
	if _, err := startLive(liveCtx, s.way, s.index, s.pages, reg != nil); err != nil {
		return err
	}
	s.pages.UseHealth(web.Health{Release: releaseName(), DeployResult: s.cfg.news.deployResult, Proxy: s.way.ProxyCounts()})
	s.pages.UseAdmins(s.cfg.access.admins)
	s.pages.UseLocalRepos(s.cfg.access.localRepos)
	return nil
}

// startBackground starts what runs beside the pages until the server stops: the usage and task indexes, routines, retention
// and the inbox.
func (s *server) startBackground() error {
	stopUsage, err := startUsage(s.way.proxy, s.pages, indexes{url: indexesOf(s.cfg.stores).url, tasks: s.index.Root()}, s.cfg.core.org)
	if err != nil {
		return err
	}
	s.started(stopUsage)
	stopTasks, err := startTasks(s.index, s.pages, s.queue.Stop, s.cfg.stores.databaseURL)
	if err != nil {
		return err
	}
	s.started(stopTasks)
	background, stopBackground := context.WithCancel(context.Background())
	s.started(stopBackground)
	news, err := routineNews(s.cfg.news)
	if err != nil {
		return err
	}
	s.pages.UseRoutineNews(news, s.cfg.core.publicURL)
	go s.pages.RunRoutines(background, time.Minute)
	if dir := s.cfg.stores.retention; dir != "" {
		policy, err := retention.LoadPolicy(dir)
		if err != nil {
			return err
		}
		s.pages.UseRetention(dir, policy)
		go s.pages.RunRetention(background, time.Duration(s.cfg.news.retentionMinutes)*time.Minute)
	}
	if dir := s.cfg.served.inbox; dir != "" {
		s.pages.UseInbox(dir)
		go s.pages.RunInbox(background, 2*time.Second)
	}
	return nil
}

// openDoors is who may come in and how: the organization's roster, the settings it is governed by, a room's live connection,
// and gusto cloud of the organization's cloud.
func (s *server) openDoors() error {
	if path := s.cfg.served.members; path != "" {
		s.pages.UseRoster(path)
	}
	s.pages.UseSettings(governing(s.set, s.cfg))
	// A room's page follows the room over a WebSocket, and falls back to asking every few seconds when it has none.
	if err := s.pages.UseLive(roomlive.Options{MaxConnections: s.cfg.news.liveConn}); err != nil {
		return err
	}
	front, err := cloudOf(s.cfg.access, s.cfg.core.org)
	if err != nil {
		return err
	}
	if front != nil {
		s.pages.UseCloud(*front)
	}
	return nil
}

// routineNews is who is told how each run of a routine ended: an address that takes a signed request, named by
// GUSTO_NOTIFY_WEBHOOK and GUSTO_NOTIFY_SECRET, and mail to the person who added the routine, set up as GUSTO_CLOUD_MAIL is by
// GUSTO_MAIL and GUSTO_MAIL_TOKEN. With neither set nothing is sent, and the tray still hears of the turn.
func routineNews(cfg newsSettings) (notify.Every, error) {
	var news notify.Every
	if cfg.webhook != "" {
		hook := notify.Webhook{URL: cfg.webhook, Secret: cfg.webhookSecret}
		if err := hook.Check(); err != nil {
			return nil, err
		}
		news = append(news, hook)
	}
	if cfg.mail != "" {
		sender, err := cloud.MailFrom(cfg.mail, cfg.outbox, cfg.mailToken)
		if err != nil {
			return nil, err
		}
		news = append(news, notify.Mail{Via: sender})
	}
	return news, nil
}
