package main

import (
	"strings"
	"testing"

	"github.com/getlatentic/gusto/cmd/internal/cmdline/clitest"
	"github.com/getlatentic/gusto/internal/settings"
)

// Every setting gusto cloud, gusto host and gusto operator read is declared once, with help: declaring one twice or with
// nothing to say panics, and help lists them all.
func TestEverySettingOfACloudsHostsIsDeclaredOnceWithHelp(t *testing.T) {
	t.Parallel()
	for name, declare := range map[string]func(*settings.Set){
		"cloud":    new(cloudSettings).declare,
		"host":     new(nodeSettings).declare,
		"operator": new(operatorSettings).declare,
	} {
		set := settings.New()
		declare(set)
		for _, p := range set.Params() {
			if !strings.HasPrefix(p.Name, "GUSTO_") || p.Text == "" {
				t.Errorf("%s: %+v", name, p)
			}
		}
	}
}

// Gusto cloud's settings read as their types, and a value that does not read is said by name rather than left at its default.
func TestCloudsSettingsReadAsTheirTypesAndAWrongOneIsSaid(t *testing.T) {
	t.Parallel()
	cfg := cloudFrom(t, map[string]string{"GUSTO_CLOUD_TRUST_PROXY": "1", "GUSTO_CLOUD_LOCAL_NODE": "on", "GUSTO_CLOUD_DEMO_MINUTES": "20"})
	if !cfg.front.trustProxy || !cfg.nodes.local || cfg.demo.forCloud().MaxOrgs != 5 || cfg.billing.seatUSD != 25 || cfg.front.addr != "127.0.0.1:8050" {
		t.Errorf("the settings read as %+v", cfg)
	}
	if cfg.signup.limits().Paid.Computers != cfg.signup.paidMembers {
		t.Error("a paying organization's workspaces are as many as its people")
	}
	wrong := map[string]string{
		"GUSTO_CLOUD_DEMO_MAX": "five", "GUSTO_CLOUD_FREE_MAIL": "yes", "GUSTO_CLOUD_SERVERS": "no",
		"GUSTO_CLOUD_NODE_LAUNCH": "gce", "GUSTO_CLOUD_STRIPE_KEY": "sk_live", "GUSTO_CLOUD_PER_DOMAIN": "0",
	}
	_, err := loadCloud(func(name string) string { return wrong[name] })
	for name := range wrong {
		if name == "GUSTO_CLOUD_STRIPE_KEY" {
			continue
		}
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s is not said in %v", name, err)
		}
	}
	if err != nil && strings.Contains(err.Error(), "sk_live") {
		t.Errorf("a secret was printed: %v", err)
	}
}

// Each command of a gusto cloud's hosts with a setting that does not read stops before it starts, with the exit code of a
// command started wrongly and the setting's name.
func TestACloudsCommandWithAWrongSettingDoesNotStart(t *testing.T) {
	for command, setting := range map[string]string{
		"cloud": "GUSTO_CLOUD_DEMO_MAX", "host": "GUSTO_NODE_PORT", "operator": "GUSTO_CLOUD_DEMO_PER_DAY",
	} {
		t.Setenv(setting, "many")
		_, stderr, code := clitest.Capture(t, func() int { return run([]string{command}) })
		if code != 2 || !strings.Contains(stderr, setting) {
			t.Errorf("gusto %s with %s wrong: exit %d, %q", command, setting, code, stderr)
		}
		t.Setenv(setting, "")
	}
}

// Billing is off with none of Stripe's settings, refused with some, and on with all three.
func TestBillingIsOffWithNoStripeSettingsAndRefusedWithSome(t *testing.T) {
	t.Parallel()
	if s, err := cloudFrom(t, nil).billing.stripe(); s != nil || err != nil {
		t.Errorf("%v %v", s, err)
	}
	if _, err := cloudFrom(t, map[string]string{"GUSTO_CLOUD_STRIPE_KEY": "sk_test"}).billing.stripe(); err == nil {
		t.Error("a key with no price and no webhook secret was taken")
	}
	all := map[string]string{"GUSTO_CLOUD_STRIPE_KEY": "sk_test", "GUSTO_CLOUD_STRIPE_PRICE": "price_seat", "GUSTO_CLOUD_STRIPE_WEBHOOK_SECRET": "whsec"}
	if s, err := cloudFrom(t, all).billing.stripe(); err != nil || s.SeatUSD != 25 {
		t.Errorf("%+v %v", s, err)
	}
}
