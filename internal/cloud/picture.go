package cloud

import (
	"fmt"
	"html/template"
)

// picture is a screen of the product, in its light and its dark look, and the browser shows the one the visitor's setting asks for.
// The pair is static/shots/<name>-light.webp and <name>-dark.webp, cut from one run of the product so that the two say the same thing.
func picture(name, alt string, width, height int) template.HTML {
	light := fingerprinted("/static/shots/" + name + "-light.webp")
	dark := fingerprinted("/static/shots/" + name + "-dark.webp")
	return template.HTML(fmt.Sprintf(
		`<picture><source media="(prefers-color-scheme: dark)" srcset="%s"><img src="%s" width="%d" height="%d" alt="%s" loading="lazy" decoding="async"></picture>`,
		dark, light, width, height, template.HTMLEscapeString(alt)))
}

// darkScreen is a screen of the product in its dark look only, for a band that is dark whatever the visitor's setting.
func darkScreen(name, alt string, width, height int) template.HTML {
	return template.HTML(fmt.Sprintf(
		`<img src="%s" width="%d" height="%d" alt="%s" loading="lazy" decoding="async">`,
		fingerprinted("/static/shots/"+name+"-dark.webp"), width, height, template.HTMLEscapeString(alt)))
}
