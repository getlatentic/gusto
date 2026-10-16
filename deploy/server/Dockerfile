# The server: `gusto serve`, which is the pages, the API and the proxy in one
# process. It starts every job in a container of its own from the runner image
# (the root Dockerfile), so it carries what that takes and no harness: git and
# ssh, because the repository is cloned out here before the box starts, over
# whichever address the job names, and a container command to start the box.
#
# Built from the repository root:
#   docker build -f deploy/server/Dockerfile -t gusto-server .
#
# Every base is pinned by digest, so a rebuild months later starts from the same
# bytes. The tag beside each digest is for the reader and is ignored when the
# digest is present. To move one, read the new digest with
#   docker buildx imagetools inspect docker.io/library/<name>:<tag>
# and change the tag and the digest together.

FROM docker.io/library/golang:1.26@sha256:6c2a5538f964f1c82f97ad14988bf05de100d922d159d0e398b54c7b0ca0c6c9 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
RUN CGO_ENABLED=0 go build -trimpath -o /gusto ./cmd/gusto

# The docker CLI speaks to a Docker daemon and to podman's Docker-compatible
# socket alike, so one client serves every runtime this install supports. The
# binary in this image is statically linked, which is why it runs on Debian.
FROM docker.io/library/docker:29-cli@sha256:018edbc908e08fcc9dbf029c812c34251e9b4719e6f71ca0e5eae2a987d014ca AS cli

# The runner's node:24-slim is Debian bookworm, so the server is too: one
# Debian release to mirror for both images.
FROM docker.io/library/debian:bookworm-slim@sha256:3783cc01769c7b2b1b83a5c5ad96c815348e28ed7da68e2e3687004faa906251
RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates openssh-client \
 && rm -rf /var/lib/apt/lists/*
COPY --from=cli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=build /gusto /usr/local/bin/gusto
# The server runs as root in its container. A job's container writes its
# evidence as root, and the server signs that evidence in the same directory
# once the box is gone, so a server under another user could not write the
# signature. What root here is worth depends on the runtime it is handed:
# deploy/README.md says so for each.
ENV GUSTO_ADDR=0.0.0.0:8040 \
    GUSTO_CONTAINER=docker
EXPOSE 8040
ENTRYPOINT ["gusto"]
CMD ["serve"]
