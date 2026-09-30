package web

import (
	"path"
	"regexp"

	"github.com/getlatentic/haas/internal/apps"
)

// previewScript is a static file server in the one thing every agent's computer has, node, so a page an agent built can be
// looked at without it or a person writing a server. It serves the folder it is given, and index.html for a folder, and
// nothing outside it.
const previewScript = `const[h,f,p]=["http","fs","path"].map(require),d=p.resolve(process.argv[1]),t={".html":"text/html",".js":"text/javascript",".css":"text/css",".json":"application/json",".svg":"image/svg+xml",".png":"image/png",".jpg":"image/jpeg",".ico":"image/x-icon"};` +
	`h.createServer((q,s)=>{let u=p.join(d,decodeURIComponent(q.url.split("?")[0]));if(u!==d&&!u.startsWith(d+p.sep)){s.writeHead(403);return s.end()}` +
	`if(f.existsSync(u)&&f.statSync(u).isDirectory())u=p.join(u,"index.html");` +
	`f.readFile(u,(e,b)=>{if(e){s.writeHead(404);return s.end("not found")}s.writeHead(200,{"Content-Type":t[p.extname(u)]||"application/octet-stream"});s.end(b)})}).listen(+process.env.PORT,"0.0.0.0")`

// previewFolder is a folder in the agent's folder, as it is named in a command.
var previewFolder = regexp.MustCompile(`^(?:\.|[A-Za-z0-9_][A-Za-z0-9._-]*(?:/[A-Za-z0-9_][A-Za-z0-9._-]*)*)$`)

// previewCommand is the line that serves a folder of the agent's folder, or nothing when the folder is not a plain
// name in it.
func previewCommand(folder string) string {
	if !previewFolder.MatchString(folder) {
		return ""
	}
	return "node -e '" + previewScript + "' " + folder
}

// previewsOf is the folders among files that hold a page: where an index.html is.
func previewsOf(files []string) []string {
	var folders []string
	for _, file := range files {
		if path.Base(file) != "index.html" {
			continue
		}
		folder := path.Dir(path.Clean(file))
		if previewFolder.MatchString(folder) && !contains(folders, folder) {
			folders = append(folders, folder)
		}
	}
	return folders
}

func contains(list []string, want string) bool {
	for _, each := range list {
		if each == want {
			return true
		}
	}
	return false
}

// previewPort is the first port from 8080 that none of the agent's apps has.
func previewPort(found []apps.App) int {
	for port := 8080; port < 8100; port++ {
		free := true
		for _, each := range found {
			if each.Port == port && (each.State == apps.Starting || each.State == apps.Live) {
				free = false
			}
		}
		if free {
			return port
		}
	}
	return 0
}

// previewing is what the page says of the agent's apps: one that answers is opened, one that is starting is waited for.
func previewing(found []apps.App) (live []apps.App, starting bool) {
	for _, each := range found {
		switch each.State {
		case apps.Live:
			live = append(live, each)
		case apps.Starting:
			starting = true
		}
	}
	return live, starting
}
