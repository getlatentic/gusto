package cloud

import (
	"fmt"
	"html/template"

	"github.com/getlatentic/gusto/internal/web"
)

// picture is a screen of the product in light and dark, one file each: static/shots/<name>-light.webp and -dark.webp.
func picture(name, alt string, width, height int) template.HTML {
	light := fingerprinted("/static/shots/" + name + "-light.webp")
	dark := fingerprinted("/static/shots/" + name + "-dark.webp")
	return template.HTML(fmt.Sprintf(
		`<picture><source media="(prefers-color-scheme: dark)" srcset="%s"><img src="%s" width="%d" height="%d" alt="%s" loading="lazy" decoding="async"></picture>`,
		dark, light, width, height, template.HTMLEscapeString(alt)))
}

// darkScreen is the dark file only, for a band that is dark in both themes.
func darkScreen(name, alt string, width, height int) template.HTML {
	return template.HTML(fmt.Sprintf(
		`<img src="%s" width="%d" height="%d" alt="%s" loading="lazy" decoding="async">`,
		fingerprinted("/static/shots/"+name+"-dark.webp"), width, height, template.HTMLEscapeString(alt)))
}

// agentMark is an agent's own mark with no box, drawn as its owner gives it. A mark that is black on a light page has the file its
// owner publishes for a dark one, or is turned white, so it shows on both.
func agentMark(name, size string) template.HTML {
	file := "/static/agents/" + name + ".svg"
	if name == "hermes" {
		file = "/static/agents/hermes.webp"
	}
	img := func(src, class string) string {
		return fmt.Sprintf(`<img class="flex-none object-contain %s %s" src="%s" alt="" aria-hidden="true">`, size, class, web.Asset(src))
	}
	switch name {
	case "opencode", "pi":
		return template.HTML(img(file, "dark:hidden") + img("/static/agents/"+name+"-dark.svg", "hidden dark:block"))
	case "codex":
		return template.HTML(img(file, "dark:invert"))
	case "hermes":
		return template.HTML(img(file, "dark:invert dark:mix-blend-screen"))
	}
	return template.HTML(img(file, ""))
}
