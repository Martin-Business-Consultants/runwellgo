# runwell-runner in a container: the runner and the tools it uses (git, gh,
# ripgrep, mise), with a slot for your coding agent's CLI. A container per
# runner keeps what an agent does to itself: it sees only this filesystem,
# the repositories the runner clones, and the credentials you pass in.
#
#   docker build -f docker/runner/Dockerfile -t runwell-runner .
#   docker run -d --name runner --restart unless-stopped \
#     -e GH_TOKEN=github_pat_… -e ANTHROPIC_API_KEY=sk-ant-… \
#     -e RUNWELL_RUNNER_TOKEN=rwm_… \
#     -v runner-config:/home/runner/.config/runwell-runner \
#     runwell-runner run
#
# The first time, set it up inside it (once, kept in the volume):
#   docker run --rm -it -v runner-config:/home/runner/.config/runwell-runner runwell-runner \
#     init --url https://runwell.example.com --token rwm_… --repos acme/website
#
# The agent: by default Claude Code (AGENT=claude-code, needs ANTHROPIC_API_KEY
# at run time). Build with --build-arg AGENT=codex for OpenAI's Codex CLI,
# --build-arg AGENT=none to add your own in a FROM runwell-runner image, and
# set the runner's agent command to match (docs/runners.md).

FROM golang:1.26.9-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=
RUN V="${VERSION:-$(cat VERSION 2>/dev/null || echo dev)}" \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${V#v}" -o /out/runwell-runner ./cmd/runwell-runner

FROM debian:bookworm-slim
ARG AGENT=claude-code
# git, ripgrep and what gh's and mise's installers need; gh from GitHub's
# own apt repository.
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl git gnupg ripgrep tini \
 && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg -o /usr/share/keyrings/githubcli-archive-keyring.gpg \
 && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list \
 && apt-get update && apt-get install -y --no-install-recommends gh \
 && rm -rf /var/lib/apt/lists/*
# mise, for the tools a repository pins (mise.toml, .tool-versions), kept
# where the runner's user can add to it.
ENV MISE_DATA_DIR=/opt/mise/data MISE_CONFIG_DIR=/opt/mise/config MISE_CACHE_DIR=/opt/mise/cache
ENV PATH=/opt/mise/data/shims:$PATH
RUN curl -fsSL https://mise.run | MISE_INSTALL_PATH=/usr/local/bin/mise sh
# The agent's slot. Node comes from mise, for the CLIs published on npm.
RUN case "$AGENT" in \
      claude-code) mise use -g --yes node@22 && mise exec -- npm install -g @anthropic-ai/claude-code ;; \
      codex) mise use -g --yes node@22 && mise exec -- npm install -g @openai/codex ;; \
      none) ;; \
      *) echo "AGENT is claude-code, codex or none" >&2; exit 1 ;; \
    esac
RUN useradd -m -u 10001 runner && mkdir -p /opt/mise && chown -R runner:runner /opt/mise
COPY --from=build /out/runwell-runner /usr/local/bin/runwell-runner
USER runner
WORKDIR /home/runner
# gh lends git its token (GH_TOKEN) for pushes.
ENV LOG_FORMAT=json
RUN git config --global user.name "Runwell runner" && git config --global user.email "runner@runwell.invalid" \
 && git config --global credential.https://github.com.helper '!gh auth git-credential'
ENTRYPOINT ["tini", "--", "runwell-runner"]
CMD ["run"]
