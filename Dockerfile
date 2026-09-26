# The runner, and the harness it runs. The runner is inside the box because a
# harness with its own tool loop runs its edits and shell in its own process, so
# containing the CLI alone would leave those outside.

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod ./
COPY cmd cmd
COPY internal internal
RUN CGO_ENABLED=0 go build -trimpath -o /haas ./cmd/haas

FROM node:24-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates \
 && rm -rf /var/lib/apt/lists/*
# One image a harness. The adapter names the program; this provides it.
ARG HARNESS_PACKAGE=opencode-ai
RUN npm install -g --no-fund --no-audit "$HARNESS_PACKAGE"
COPY --from=build /haas /usr/local/bin/haas
WORKDIR /job
ENTRYPOINT ["haas"]
