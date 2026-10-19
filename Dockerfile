# Runwell in a container: the one binary (API, app, portal, SQLite, MCP, the
# default plugins, all built in) on a small image. Everything it keeps lives
# in /data: mount a named volume there, one per site. It refuses to start
# without one (EPHEMERAL_DATA=1 allows a trial): with no volume, or Docker's
# anonymous one, every deploy would start the site afresh. There's no VOLUME
# line for that reason: it would make Docker hand each container a new one.
#
#   docker build -t runwell .
#   docker run -p 8080:80 -v runwell-data:/data -e APP_URL=https://runwell.example.com runwell
#
# Settings are environment variables (README): APP_URL, SECRET_KEY,
# TRUST_PROXY=1 behind the host's proxy, PORT (80 here), LOG_FORMAT.
# The first owner: the sign-up screen on a fresh install, or
# `docker exec -it <container> runwell user create --email … --name … --password -`.
# Health check: GET /up.

FROM golang:1.26.9-alpine AS build
WORKDIR /src
# The binary is cmd/runwell, a module of its own: this repository's core
# (its go.mod replaces the core with ../..) and the official plugins, at the
# versions its go.mod names. go.work is for local development, not this.
ENV GOWORK=off
COPY go.mod go.sum ./
COPY cmd/runwell/go.mod cmd/runwell/go.sum ./cmd/runwell/
RUN cd cmd/runwell && go mod download
COPY . .
# The release this is: the VERSION file, unless the build passes one
# (Hoster and other hosts that build from the repository pass none).
ARG VERSION=
RUN V="${VERSION:-$(cat VERSION 2>/dev/null || echo dev)}" \
 && cd cmd/runwell \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${V#v}" -o /out/runwell .

# Alpine for CA certificates (SMTP, webhooks, deploy hooks) and su-exec: the
# container starts as root only to hand /data to the app user, whatever owns
# the volume the host mounts, then runs as app (docker/runwell).
FROM alpine:3.22
RUN apk add --no-cache ca-certificates su-exec tzdata \
 && addgroup -S -g 10001 app && adduser -S -D -H -u 10001 -G app app \
 && mkdir -p /data && chown app:app /data
COPY --from=build /out/runwell /usr/local/libexec/runwell
COPY docker/runwell /usr/local/bin/runwell
RUN chmod 0755 /usr/local/bin/runwell
# Port 80: what Kamal-style hosts (kamal-proxy) send traffic and health
# checks to by default. A host that sets PORT moves it.
ENV DATA_DIR=/data LOG_FORMAT=json PORT=80
EXPOSE 80
ENTRYPOINT ["runwell"]
CMD ["serve"]
