# The built web app

`web/build.sh` builds the React app in `web/` into `dist/` here, and the gusto binary embeds it. `dist/` is not committed: CI,
the release and the Docker images build it from source. Run `web/build.sh` before `go build ./cmd/gusto` or `go test ./internal/web`.
