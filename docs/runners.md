# Runners

Runners hand code and other heavy work from Runwell to a machine you control. Runwell itself never
runs a shell command: a person (or their AI agent) hands a piece of work to a runner, a small
program on a server, VM or container of yours pulls the job, does it in a fresh checkout with your
coding agent (Claude Code, Codex, aider, a script), streams its progress back, and finishes with a
branch, a pull request and a summary. A person then approves or declines it in Runwell; only an
approval makes Runwell act on it.

It's two pieces:

- **The Runners plugin** (`plugins/runners`, screens in `ui/src/plugins/runners`): Settings ›
  Runners, the jobs at `/runners` with each job's live log, a **Code** tab and a **Hand to a runner**
  quick action on every piece of work, and agent tools.
- **`runwell-runner`** (`cmd/runwell-runner`): the program on your machine. `make build` and
  `make dist` build it beside `runwell`, and each release attaches it for every platform.

## Setting one up

1. Install the plugin: Settings › Plugins › Runners › Install, then switch it on (it asks for its own
   tables, records, live updates, notifications, acting as the person who hands work over, and
   tokens for machines).
2. Settings › Runners › **Add a runner**: name it. Runwell shows its token once, with the commands
   to run on the machine:

   ```sh
   curl -fsSL https://raw.githubusercontent.com/Martin-Business-Consultants/runwellgo/main/install.sh | RUNWELL_BINARY=runwell-runner sh
   runwell-runner init --url https://runwell.example.com --token rwm_… --repos acme/website,acme/*
   runwell-runner doctor
   runwell-runner run
   ```

3. On the machine: `gh auth login` (and `gh auth setup-git`, so `git push` uses it), install your
   coding agent's CLI and sign it in, then `runwell-runner doctor` checks git, gh (signed in),
   ripgrep, mise, the agent command and that Runwell answers. `runwell-runner run` starts it; the
   dialog in Settings › Runners says when it has connected.
4. Optionally, set a **default repository** for a client or an engagement (Settings › Runners), so
   handing its work over starts with the right one.

Then open a piece of work, **Code › Hand this to a runner**: the repository (the default), the
instructions (its title and description to start), and a runner, or any.

### The config file

`runwell-runner init` writes `~/.config/runwell-runner/config.json` (`--config` or
`RUNWELL_RUNNER_CONFIG` for another), readable only by its user:

| Key | |
| --- | --- |
| `url`, `token` | Runwell's address and the runner's token (`RUNWELL_RUNNER_TOKEN` overrides the file's, for a secret store) |
| `work_dir` | where each job's checkout is made, and removed after |
| `allowed_repos` | the only repositories it touches: `owner/name`, `owner/*`, or a local path |
| `concurrency` | jobs at once (1) |
| `agent_command` | the agent, run with `sh -c` in the checkout (below) |
| `timeout_minutes` | a cap of its own on a job, below Runwell's (each job has one, 60 minutes unless set) |
| `no_push`, `no_pr` | as the flags below |

`runwell-runner run` flags: `--once` (one job, then exit), `--no-push` (commit locally, push nothing:
a dry run), `--no-pr` (push the branch, open no pull request: a remote that isn't GitHub),
`--log-format text|json` (to stderr). SIGTERM or Ctrl-C stops it cleanly: no new jobs, the agent in
hand is stopped and its job reported as failed, its directory removed; a second signal ends it at
once.

## The agent command

The command is run with `sh -c` in the checkout, with these replaced (each shell-quoted):
`{prompt_file}` (the prompt: the instructions, the work's brief from Runwell, and how to finish),
`{workdir}` (the checkout), `{branch}` and `{job_id}`. It also gets `RUNWELL_PROMPT_FILE`,
`RUNWELL_SUMMARY_FILE`, `RUNWELL_BRANCH`, `RUNWELL_WORKDIR`, `RUNWELL_REPO`, `RUNWELL_JOB_ID` and
`RUNWELL_WORK_URL`, never the runner's token. Its stdout and stderr are the job's log. The summary
is what it writes to `$RUNWELL_SUMMARY_FILE`, else its last message (a Claude Code `result`, or its
last lines). It may commit or leave changes in the tree; the runner commits what's left, pushes and
opens the pull request. A non-zero exit fails the job.

- **Claude Code** (the default):
  `claude -p --output-format stream-json --verbose --permission-mode acceptEdits < {prompt_file}`.
  The log shows what it says and each tool it uses. Add `--allowedTools "Bash(npm test:*)"` and the
  like for the commands it may run without asking, or `--dangerously-skip-permissions` only inside a
  throwaway container. Needs `ANTHROPIC_API_KEY` (or a signed-in `claude`).
- **Codex**: `codex exec --full-auto "$(cat {prompt_file})"`.
- **aider**: `aider --yes-always --message-file {prompt_file}`.
- **A script**: anything that reads the prompt and edits the tree, such as
  `./scripts/do-it.sh {prompt_file}`.
- A repository whose tools come from mise: the runner runs `mise install` first when it has a
  `mise.toml` or `.tool-versions`; prefix the command with `mise exec --` to run the agent with them.

## What happens to a job

`queued` → `claimed` (a runner took it) → `running` (its first log lines) → then:

- **awaiting approval**: it changed files. The person who asked is notified, and the approval is
  raised for the AI plugin's **Approvals** (below); the job page has **Approve** and **Decline**
  too. **Approved**: Runwell adds a note to the work with the pull request and summary, and moves
  the work to in review. **Declined**: a note says so, and on its next heartbeat the runner closes
  the pull request and deletes the branch (`gh pr close --delete-branch`, or `git push --delete`).
- **succeeded**: it finished with nothing to change.
- **failed**: the agent failed, the time limit passed, the runner refused the repository, or the
  runner stopped answering (a job claimed or running past its limit, or whose runner sent no
  heartbeat for three minutes, is failed by a task every minute). The person who asked is notified.
- **cancelled**: by the person who asked (or someone who manages runners). A running agent is
  stopped within a few seconds; a job awaiting approval has its pull request closed.

**Try again** queues a copy. Cancelling or retrying a job awaiting approval withdraws its approval.

### Approvals

Runwell's Approvals (part of the AI plugin) is where work done elsewhere is decided. A finished job
with changes emits the plugin event `runners.approval.requested` with:

```json
{"id": "12", "kind": "runner_job", "title": "…", "summary": "…", "record": "Todo:18",
 "link": "https://github.com/acme/website/pull/4", "details_markdown": "files changed, duration, runner, the agent's summary",
 "requested_by": 11, "approvers": {"user_ids": [11]},
 "approve": {"tool": "approve_runner_job", "args": {"job_id": 12}},
 "decline": {"tool": "decline_runner_job", "args": {"job_id": 12}}}
```

and `runners.approval.withdrawn` `{id}` when it's cancelled or retried first. Approvals runs the
tool named as the deciding person. Where that plugin isn't on, the job page's buttons do the same.

## Agent tools

`list_runners`, `start_runner_job` (`record`, `repo`, `instructions`, `base_branch`, `runner_id`;
`repo` defaults to the engagement's or client's), `list_runner_jobs`, `show_runner_job`,
`cancel_runner_job`, `approve_runner_job` and `decline_runner_job` (`job_id`, `note`). Starting a job
is a change, so an AI agent's request to start one goes through the usual approval first.

## The protocol

A runner calls the plugin's machine routes, under `/api/x/runners/machine`, with
`Authorization: Bearer rwm_…`. The core checks the token against those this plugin issued (only a
digest is kept) before the plugin sees the call; nothing else in Runwell takes it.

| Route | Body | Answers |
| --- | --- | --- |
| `POST /heartbeat` (every 15 s) | `{version, os, arch, hostname, tools: {git: "2.45.0", …}, agent, allowed_repos, busy, max_jobs}` | `{name, paused, poll_seconds, cancel: [job ids], cleanup: [{job_id, repo, branch, pr_url}]}` |
| `POST /claim` (every 5 s while free) | `{allowed_repos}` | `{job: null}` or `{job: {id, record, title, repo, base_branch, branch, instructions, brief, timeout_minutes, work_url, job_url, requested_by}}` |
| `POST /jobs/{id}/events` (about every second) | `{events: [{stream: stdout\|stderr\|runner, text, at}]}` | `{cancel, status}` |
| `GET /jobs/{id}/cancel` (between quiet batches) | | `{cancel, status}` |
| `POST /jobs/{id}/finish` | `{outcome: changes\|no_changes\|failed\|refused\|cancelled, pr_url, branch, summary, files_changed, duration_seconds, error}` | `{status}` |
| `POST /jobs/{id}/cleaned` | `{error}` (blank when it worked) | `{cleanup}` |

A claim is atomic: one job goes to one runner, and only a job whose repository the runner allows
(its own list, as it last reported, and Runwell's list for it when there is one), for any runner or
that one. A runner sees and reports on only the jobs it claimed. A job's log keeps its last 5,000
lines.

## Security

- **Tokens.** Each runner has its own token (`rwm_…`), shown once; Runwell keeps only its SHA-256
  digest. It reaches only the Runners plugin's machine routes: not the API, not MCP, not another
  plugin's. Removing a runner revokes its token at once; switching the plugin off or removing it
  stops every runner's. A person's token or session never reaches a machine route.
- **Repositories, on both sides.** Runwell hands a runner a job only for a repository it allows
  (and, when you set one, Runwell's own list for that runner). The runner checks its own allowlist
  again before it clones anything, and refuses the job otherwise, whatever Runwell says: a
  compromised Runwell can't point a runner at a repository you didn't list on that machine.
- **Isolation.** The agent runs as the runner's user and can do whatever that user can, including
  running code from the repository. Run the runner in a container or VM of its own, with only the
  credentials it needs: a GitHub token scoped to the repositories it allows (a fine-grained token
  with contents and pull requests), and the agent's API key. `docker/runner/Dockerfile` builds an
  image with git, gh, ripgrep, mise and a slot for the agent's CLI (Claude Code by default,
  `--build-arg AGENT=codex` or `none`):

  ```sh
  docker build -f docker/runner/Dockerfile -t runwell-runner .
  docker run --rm -it -v runner-config:/home/runner/.config/runwell-runner runwell-runner \
    init --url https://runwell.example.com --token rwm_… --repos acme/website
  docker run -d --restart unless-stopped -e GH_TOKEN=… -e ANTHROPIC_API_KEY=… \
    -v runner-config:/home/runner/.config/runwell-runner runwell-runner run
  ```

- **What gets sent.** To the runner: the repository, branch names, the instructions and the work's
  brief (its engagement, agreed scope, recent notes and open commitments, as the person who handed
  it over may read them). Back to Runwell: the agent's output, line by line, with the runner's
  token, the values of its secret-looking environment variables (`*TOKEN*`, `*KEY*`, `*SECRET*`,
  `*PASSWORD*`) and anything shaped like a known token (GitHub, Anthropic, OpenAI, Slack, AWS)
  replaced with `[redacted]` (Runwell redacts token shapes again on arrival); the summary, the
  files changed and the pull request's address. The code itself stays between the runner and
  GitHub.
- Each job is a fresh clone in a new directory under `work_dir`, removed when the job ends.

## Trying it without GitHub

The runner takes a local path as a repository, cloned with `git` rather than `gh`, and `--no-pr`
pushes the branch without opening a pull request (logging the one it would have opened):

```sh
git init --bare -b main /tmp/site.git   # and push a first commit to it
runwell-runner init --url http://localhost:8080 --token rwm_… --repos /tmp/site.git \
  --agent './fake-agent.sh {prompt_file}' --work-dir /tmp/runner-work
runwell-runner run --no-pr
```

Set the engagement's default repository to `/tmp/site.git` in Settings › Runners, then hand a piece
of its work over. `--no-push` keeps even the branch on the machine.
