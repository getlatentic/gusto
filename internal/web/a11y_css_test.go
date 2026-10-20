package web

import (
	"io/fs"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	themeBlock  = regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`)
	themeColor  = regexp.MustCompile(`--gu-([a-z0-9-]+):\s*#([0-9A-Fa-f]{6})\b`)
	themeAccent = regexp.MustCompile(`data-accent="([a-z]+)"`)
)

// The focus ring is drawn in --gu-focus with nothing between it and the surface it sits on, so it has 3 to 1 against every surface a
// control is drawn on, in each theme and each accent.
func TestTheFocusRingStandsOutFromEverySurface(t *testing.T) {
	colors := themeColors(t)
	for _, theme := range []string{"light", "dark"} {
		for accent := range colors["light"] {
			if accent == "" {
				continue
			}
			focus := themeLookup(colors, theme, accent, "focus")
			for _, surface := range []string{"rail", "bg-subtle", "bg", "surface"} {
				if ratio := contrast(focus, themeLookup(colors, theme, accent, surface)); ratio < 3 {
					t.Errorf("%s %s: the focus ring #%s is %.2f to 1 on %s", theme, accent, focus, ratio, surface)
				}
			}
		}
	}
}

// themeColors reads theme.css into colors by theme, then by accent ("" for the rules that name none), then by token.
func themeColors(t *testing.T) map[string]map[string]map[string]string {
	t.Helper()
	body, err := fs.ReadFile(assets, "static/theme.css")
	if err != nil {
		t.Fatal(err)
	}
	colors := map[string]map[string]map[string]string{"light": {}, "dark": {}}
	for _, block := range themeBlock.FindAllStringSubmatch(cssComment.ReplaceAllString(string(body), ""), -1) {
		for _, selector := range strings.Split(block[1], ",") {
			theme, accent := "light", ""
			if strings.Contains(selector, `data-theme="dark"`) {
				theme = "dark"
			}
			if named := themeAccent.FindStringSubmatch(selector); named != nil {
				accent = named[1]
			}
			if colors[theme][accent] == nil {
				colors[theme][accent] = map[string]string{}
			}
			for _, color := range themeColor.FindAllStringSubmatch(block[2], -1) {
				colors[theme][accent][color[1]] = color[2]
			}
		}
	}
	return colors
}

// themeLookup is a token as the page resolves it: the accent's own rule, then the theme's, then the light theme's.
func themeLookup(colors map[string]map[string]map[string]string, theme, accent, token string) string {
	for _, rule := range []map[string]string{colors[theme][accent], colors[theme][""], colors["light"][""]} {
		if hex, ok := rule[token]; ok {
			return hex
		}
	}
	return "000000"
}

func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

func luminance(hex string) float64 {
	weights := []float64{0.2126, 0.7152, 0.0722}
	total := 0.0
	for i, weight := range weights {
		channel, _ := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		c := float64(channel) / 255
		if c <= 0.03928 {
			c /= 12.92
		} else {
			c = math.Pow((c+0.055)/1.055, 2.4)
		}
		total += weight * c
	}
	return total
}

var (
	motionRule     = regexp.MustCompile(`@media\(prefers-reduced-motion:reduce\)\{\*,(?:\*::before|:before),(?:\*::after|:after)\{([^}]*)\}`)
	motionDuration = regexp.MustCompile(`(?:transition|animation)[a-z-]*:[^;}]*?(\d*\.?\d+)(ms|s)\b`)
)

// Motion is short, and none of it is left for a person who asks for less. The spinner turns for as long as a turn runs, and is exempt.
func TestMotionIsShortAndOptional(t *testing.T) {
	app, err := fs.ReadFile(assets, "static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	rule := motionRule.FindStringSubmatch(strings.NewReplacer(" ", "", "\n", "", "\t", "").Replace(string(app)))
	if rule == nil || !strings.Contains(rule[1], "transition-duration:0s") || !strings.Contains(rule[1], "animation-duration:0s") {
		t.Fatal("app.css does not stop every transition and animation under prefers-reduced-motion")
	}
	err = fs.WalkDir(assets, "static", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".css") || strings.HasSuffix(path, "tokens.css") {
			return err
		}
		body, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		for _, found := range motionDuration.FindAllStringSubmatch(cssComment.ReplaceAllString(string(body), ""), -1) {
			value, _ := strconv.ParseFloat(found[1], 64)
			if found[2] == "s" {
				value *= 1000
			}
			if value > 320 && !strings.Contains(found[0], "spin") && !strings.Contains(found[0], "delay") {
				t.Errorf("%s: %s lasts longer than 320ms", path, found[0])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

var outlineRemoved = regexp.MustCompile(`outline:\s*(none|0)\s*(;|$)`)

// A rule takes the outline away only where the box around the field draws the ring in its place.
func TestNoRuleHidesTheFocusRingWithoutAnother(t *testing.T) {
	ringedByTheirBox := map[string]bool{".composer textarea:focus-visible": true, ".role-list .search input": true}
	err := fs.WalkDir(assets, "static", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".css") || strings.HasSuffix(path, "tokens.css") {
			return err
		}
		body, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		for _, block := range themeBlock.FindAllStringSubmatch(cssComment.ReplaceAllString(string(body), ""), -1) {
			selector := strings.TrimSpace(block[1])
			if outlineRemoved.MatchString(block[2]) && !ringedByTheirBox[selector] {
				t.Errorf("%s: %s takes the focus ring away", path, selector)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
