# The runner, and the harnesses it runs. The runner is inside the box because a
# harness with its own tool loop runs its edits and shell in its own process, so
# containing the CLI alone would leave those outside.
#
# Image names carry their registry because podman refuses a short name unless
# the host configures a search registry, and a deploy should not need a global
# file on a shared box to build.

FROM docker.io/library/golang:1.26 AS build
WORKDIR /src
# go.sum comes with go.mod: a build that has one without the other refuses every
# dependency it cannot check, and downloading here keeps the modules on a layer
# that source edits do not invalidate.
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
ARG RELEASE=
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-X main.release=${RELEASE}" -o /gusto ./cmd/gusto

FROM docker.io/library/node:24-slim
# ripgrep is what opencode's glob and grep tools run. It downloads one on first
# use, which a box with no network cannot, and every search then fails.
RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates ripgrep \
 && rm -rf /var/lib/apt/lists/*
# HARNESSES is the list this image carries: each member is an adapter's name, an
# equals sign, and the npm package that provides its program, so the name a job
# asks for and the package that answers it are written down together. A version
# pins the same way it does for npm: codex=@openai/codex@0.1.0.
ARG HARNESSES="opencode=opencode-ai"
RUN set -eu; \
    packages=""; \
    for member in $HARNESSES; do \
      case "$member" in \
        ?*=?*) packages="$packages ${member#*=}" ;; \
        *) echo "HARNESSES wants name=package, and $member is not" >&2; exit 1 ;; \
      esac; \
    done; \
    if [ -n "$packages" ]; then npm install -g --no-fund --no-audit $packages; fi
# The image says what it carries, so `docker image inspect` and the server's
# catalog both read it without anybody opening this file. The catalog still
# asks each program's PATH, because a label is a claim and the PATH is what a
# job will meet.
LABEL com.getlatentic.gusto.harnesses="$HARNESSES"
# MCP_SERVERS is the npm packages of the MCP servers a setup may give a
# harness. The box fetches nothing once it is sealed, so a server a setup names
# is one the image already carries.
ARG MCP_SERVERS=""
RUN if [ -n "$MCP_SERVERS" ]; then npm install -g --no-fund --no-audit $MCP_SERVERS; fi
LABEL com.getlatentic.gusto.mcp="$MCP_SERVERS"
COPY --from=build /gusto /usr/local/bin/gusto
# A harness's own configuration is not the adapter's: the adapter has to stay
# portable between a person's computer and a container. Each harness has a
# directory of its own under internal/harness/config/, named after it and copied
# to /root/.config/<name>/, so two harnesses never share a file; on a person's
# computer gusto puts the same files in the harness's home. What a harness
# keeps between turns, its sessions and settings, is under /root/.local/share,
# the instance's own state; each adapter points its harness there.
COPY internal/harness/config/ /root/.config/
# The same files where a sandbox on a gateway can read them: its user has a home
# of its own and cannot read /root, and copies these into it before a harness starts.
COPY internal/harness/config/ /opt/gusto/harness/
# The harness runs as node (uid 1000, which the base image has) and the runner
# as root, so only the runner can write the record in /job/out. The runner
# gives the harness its workspace and /root before starting it.
ENV GUSTO_AGENT_UID=1000
WORKDIR /job
ENTRYPOINT ["gusto"]
