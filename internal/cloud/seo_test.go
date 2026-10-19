package cloud

import (
	"encoding/json"
	"encoding/xml"
	"html"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestTheLandingPageTellsASearchEngineWhatItIsAndWhereItLives(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		status, page, header := f.browser().get2("/")
		if status != http.StatusOK {
			t.Fatalf("%d", status)
		}
		for _, want := range []string{
			`<meta name="robots" content="index,follow,max-image-preview:large">`,
			`<link rel="canonical" href="http://cloud.test/">`,
			`<meta property="og:image" content="http://cloud.test/static/og.png">`,
			`<meta property="og:image:width" content="1200">`,
			`<meta name="twitter:card" content="summary_large_image">`,
			`<meta property="og:title" content="Latentic Agent Cloud — one place for every AI agent your team needs">`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("the landing page lacks %s", want)
			}
		}
		if description := regexp.MustCompile(`<meta name="description" content="([^"]*)"`).FindStringSubmatch(page); description == nil || !strings.Contains(html.UnescapeString(description[1]), "Try it for 30 minutes") {
			t.Errorf("the description: %v", description)
		}
		if got := header.Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
			t.Errorf("the page lost its policy: %q", got)
		}
	})
}

func TestTheLandingPagesStructuredDataIsValidAndNamesOnlyWhatThePageSays(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		_, page, _ := f.browser().get2("/")
		found := regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`).FindStringSubmatch(page)
		if found == nil {
			t.Fatal("no structured data")
		}
		var data struct {
			Context string           `json:"@context"`
			Graph   []map[string]any `json:"@graph"`
		}
		if err := json.Unmarshal([]byte(found[1]), &data); err != nil {
			t.Fatalf("%v\n%s", err, found[1])
		}
		types := map[string]map[string]any{}
		for _, node := range data.Graph {
			types[node["@type"].(string)] = node
		}
		if data.Context != "https://schema.org" || types["Organization"]["name"] != "Latentic" || types["SoftwareApplication"]["name"] != "Latentic Agent Cloud" {
			t.Errorf("%+v", data)
		}
		for _, claimed := range []string{"offers", "aggregateRating", "price", "reviewCount"} {
			if _, found := types["SoftwareApplication"][claimed]; found {
				t.Errorf("the data claims %s, which the page does not", claimed)
			}
		}
		if strings.Contains(strings.TrimSpace(found[1]), "</script") {
			t.Error("the data can end its own script")
		}
	})
}

func TestOnlyTheLandingPageAndTheDemoFormMayBeListed(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		_, form, _ := f.browser().get2("/demo")
		if !strings.Contains(form, `content="index,follow`) || !strings.Contains(form, `<link rel="canonical" href="http://cloud.test/demo">`) || !strings.Contains(form, "Start a demo of Latentic Agent Cloud") {
			t.Errorf("the demo form is not listed with its own address and title:\n%s", form)
		}
		for _, path := range []string{"/signin", "/signup"} {
			_, page, _ := f.browser().get2(path)
			if !strings.Contains(page, `<meta name="robots" content="noindex,nofollow">`) || strings.Contains(page, "canonical") {
				t.Errorf("%s may be listed", path)
			}
		}
		b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
		_, wait, _ := b.get2("/demo/wait")
		f.server.Wait()
		if !strings.Contains(wait, `content="noindex,nofollow"`) {
			t.Errorf("a visitor's wait page may be listed:\n%.700s", wait)
		}
	})
}

func TestRobotsAndTheSitemapAnswerForTheFrontDoorAndNameOnlyWhatIsWorthFinding(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		status, robots, header := f.browser().get2("/robots.txt")
		if status != http.StatusOK || !strings.HasPrefix(header.Get("Content-Type"), "text/plain") {
			t.Fatalf("%d %v", status, header)
		}
		for _, want := range []string{"User-agent: *", "Allow: /\n", "Disallow: /signin", "Disallow: /org", "Disallow: /demo/wait", "Disallow: /demo/ended/", "Sitemap: http://cloud.test/sitemap.xml"} {
			if !strings.Contains(robots, want) {
				t.Errorf("robots.txt lacks %q:\n%s", want, robots)
			}
		}
		if strings.Contains(robots, "Disallow: /demo\n") || strings.Contains(robots, "Disallow: /\n") {
			t.Errorf("robots.txt hides the demo form or everything:\n%s", robots)
		}
		_, body, header := f.browser().get2("/sitemap.xml")
		var urls struct {
			URL []struct {
				Loc string `xml:"loc"`
			} `xml:"url"`
		}
		if err := xml.Unmarshal([]byte(body), &urls); err != nil || !strings.HasPrefix(header.Get("Content-Type"), "application/xml") {
			t.Fatalf("%v\n%s", err, body)
		}
		if len(urls.URL) != 2 || urls.URL[0].Loc != "http://cloud.test/" || urls.URL[1].Loc != "http://cloud.test/demo" {
			t.Errorf("%+v", urls)
		}
		none := newFront(t, Settings{}, nil)
		if _, body, _ := none.browser().get2("/sitemap.xml"); strings.Contains(body, "/demo") {
			t.Errorf("a host with no demo lists one:\n%s", body)
		}
	})
}

func TestEveryOrganisationsAddressAndEveryAppIsKeptOutOfSearch(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := withServers(t)
		b := f.signedUp("ada@acme.com")
		someApp := strings.Repeat("a", 10)
		for _, host := range []string{"acme-com.cloud.test", "acme-com--" + someApp + ".cloud.test"} {
			status, body, header := b.at(host, "GET", "/robots.txt", nil)
			if status != http.StatusOK || body != "User-agent: *\nDisallow: /\n" || header.Get("X-Robots-Tag") != "noindex, nofollow" {
				t.Errorf("%s: %d %q %v", host, status, body, header)
			}
			if _, _, header := b.at(host, "GET", "/", nil); header.Get("X-Robots-Tag") != "noindex, nofollow" {
				t.Errorf("%s: a page of it may be listed: %v", host, header)
			}
		}
	})
}
