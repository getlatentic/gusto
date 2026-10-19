package cloud

import (
	"fmt"
	"html/template"
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
