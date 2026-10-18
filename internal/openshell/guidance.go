package openshell

// networkGuidance is the skill every agent in a sandbox is given, so that what it builds works
// where it will run. An app the agent builds is started by a person with a command they give, in
// the agent's folder, and colleagues open it through the organisation's pages, so what the agent
// does not know about this network is what its first attempt gets wrong.
const networkGuidance = `---
name: gusto-sandbox
description: How this sandbox's network works and how a web app you build is shared. Read it before you build a web app, a server, or anything that listens on a port or calls the network.
---

# The sandbox's network

- **A web app you build is shared for you.** The person you work for starts it with one command and
  gives it a port. Colleagues open it through their organisation's pages. When you finish, say the
  one line that starts the app, and the port.
- **Listen on the port in the ` + "`PORT`" + ` environment variable**, and on ` + "`0.0.0.0`" + `, which ` + "`HOST`" + ` is also
  set to. Node's ` + "`listen(port)`" + ` listens on ` + "`::`" + ` and resets IPv4 clients here, so write
  ` + "`listen(port, '0.0.0.0')`" + `. Python's ` + "`http.server`" + ` needs ` + "`--bind 0.0.0.0`" + `.
- **` + "`localhost`" + ` does not resolve.** Write ` + "`127.0.0.1`" + `.
- **Use relative links.** The app is served under a path, so write ` + "`style.css`" + ` and ` + "`fetch('api/items')`" + `,
  and not ` + "`/style.css`" + ` or ` + "`fetch('/api/items')`" + `.
- **The page runs with no cookies and no local storage of its own.** Keep what it needs on the server
  side, in memory or in a file in your folder.
- **There is no network but the model.** Everything else is refused, so nothing can be installed
  with npm or pip, and nothing loaded from a CDN. Use what is already here, such as Node's and
  Python's standard libraries, and write the page's CSS and scripts into files of your own.
`

// equipGuidance is the shell that puts the guidance where the agent finds skills.
const equipGuidance = `mkdir -p "$1/gusto-sandbox" && printf '%s' "$2" > "$1/gusto-sandbox/SKILL.md"`
