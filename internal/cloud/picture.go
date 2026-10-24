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
		return fmt.Sprintf(`<img class="object-contain %s %s" src="%s" alt="" aria-hidden="true">`, size, class, web.Asset(src))
	}
	var drawn string
	switch name {
	case "opencode", "pi":
		drawn = img(file, "dark:hidden") + img("/static/agents/"+name+"-dark.svg", "hidden dark:block")
	case "codex":
		drawn = img(file, "dark:invert")
	case "hermes":
		// its file has a white ground, which multiplies away on a light page and screens away when inverted on a dark one
		drawn = img(file, "mix-blend-multiply dark:invert dark:mix-blend-screen")
	default:
		drawn = img(file, "")
	}
	// every mark sits in a slot of one size, centered, so marks of different shapes line up
	return template.HTML(`<span class="inline-grid size-7 flex-none place-items-center">` + drawn + `</span>`)
}
