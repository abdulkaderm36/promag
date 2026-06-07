# ProMag

ProMag is a Bubble Tea-based project management TUI written in Go. It is built for keyboard-first task tracking, but also supports mouse interaction, per-project SQLite persistence, archive workflows, batch task capture, and in-app project switching.

## What It Does

- Manage team members and tasks in a terminal UI
- Switch between Tasks, Team, Timeline, Archive, and Help views
- Track task status, due dates, comments, tags, priority, and assignees
- Archive completed tasks without deleting them
- Capture multiple tasks quickly from plain-text notes
- Persist each project locally in its own SQLite database file
- Switch between projects and create new ones inside the TUI
- Export and import project JSON backups
- Collaborate through a single-project server or multi-project cloud hub
- Track active collaborators, recent activity, and stale-write conflicts

## Requirements

- Go 1.26+
- A UTF-8 terminal with mouse support

## Quick Start

Run the app from the project root:

```bash
go run .
```

This creates a local `.promag/` directory with:

- `registry.sqlite3` for the project registry and last-opened project
- `projects/*.sqlite3` for per-project data and settings

If legacy `promag.sqlite3`, `promag-data.json`, or `promag-config.json` files are present, they are imported automatically into a default project on first run.

## Build And Install

For local development:

```bash
go build
./promag
```

To install the binary into your Go bin directory:

```bash
go install .
```

Then run it as:

```bash
promag
```

## CLI Flags

- `--debug`
  - Enables mouse debug logging to `/tmp/promag-mouse.log` unless `PROMAG_DEBUG_MOUSE` is set
- `--debug-hitboxes`
  - Opens the lightweight on-screen mouse hitbox debug panel
- `--export <project-id-or-name> <path.json>`
  - Exports a project to a portable JSON file
- `--import <project-name> <path.json>`
  - Imports a project JSON file as a new local project with the given name
- `--serve <project-id-or-name>`
  - Serves a project over a token-authenticated HTTP API for collaboration clients
- `--cloud`
  - Serves all projects from a cloud data directory over project-scoped collaboration APIs
- `--cloud-init`
  - Writes a config file (`promag.env`) into the data directory with the admin token and public URL, generating a strong token if none is set. Handy for the binary/no-Docker setup; the Docker setup uses a `.env` file instead (see below).
- `--cloud-admin`
  - Opens an interactive admin TUI to browse projects, create projects, and mint or revoke per-user tokens (with connection strings) without long commands
- `--cloud-create <project-name>`
  - Creates a project in the cloud data directory
- `--cloud-import <project-name> <path.json>`
  - Imports a project JSON file into the cloud data directory
- `--cloud-token <project-id-or-name>`
  - Creates and prints a cloud project access token
- `--token-label <label>`
  - Optional label for tokens printed by `--cloud-create`, `--cloud-import`, or `--cloud-token`
- `--cloud-tokens <project-id-or-name>`
  - Lists cloud project access token IDs, labels, and status
- `--cloud-revoke-token <token-id>`
  - Revokes one cloud project access token by token ID
- `--cloud-backup <path.json>`
  - Backs up all cloud projects to a JSON file, including project token hashes
- `--cloud-restore <path.json>`
  - Restores all cloud projects from a JSON backup into an empty cloud data directory
- `--data-dir <path>`
  - Data directory for cloud commands; defaults to `./data`. In Docker this is the mounted volume, so you never pass it.
- `--addr <host:port>`
  - Address for `--serve` or `--cloud`; defaults to `:8080`
- `--token <token>`
  - Admin/bearer token for `--serve` or `--cloud`. Resolved as flag → `PROMAG_SERVER_TOKEN` env → `promag.env` in the data directory. In Docker the token comes from your `.env` file (injected as `PROMAG_SERVER_TOKEN`).
- `--public-url <url>`
  - Public base URL used in the connection strings printed by the cloud commands, for example `https://promag.example.com`. Resolved as flag → `PROMAG_PUBLIC_URL` env → `promag.env`, defaulting to the address from `--addr`. In Docker it comes from your `.env` file.

`--cloud-create`, `--cloud-import`, and `--cloud-token` print a **connection string** (an invite). A teammate pastes that one string into ProMag and the remote URL and token are filled in for them — there is normally no need to set `PROMAG_REMOTE_TOKEN` by hand. The token is stored locally with the project. `PROMAG_REMOTE_TOKEN` still works as an override, and `PROMAG_REMOTE_ACTOR` controls the collaborator ID sent with refreshes and writes.

Examples:

```bash
go run . --debug
go run . --debug-hitboxes
go run . --debug --debug-hitboxes
go run . --export Ops backups/ops.json
go run . --import "Restored Project" backups/ops.json
go run . --serve Ops --addr :8080 --token "$PROMAG_SERVER_TOKEN"
go run . --cloud-init --public-url https://promag.example.com   # writes ./data/promag.env
go run . --cloud-create Ops
go run . --cloud-import Ops backups/ops.json
go run . --cloud-token Ops --token-label "manager laptop"
go run . --cloud-tokens Ops
go run . --cloud-revoke-token tok_example
go run . --cloud-backup backups/cloud.json
go run . --cloud-restore backups/cloud.json --data-dir ./restored   # restore needs an empty dir
go run . --cloud --addr :8080
```

## Configuration

Behavior settings are stored inside each project's SQLite database.
Settings use the same optimistic version checks as tasks and members when saved through the TUI or HTTP API. If another client saves settings first, the local client reloads the latest version instead of overwriting it.

Current settings:

- `left_wheel_mode`
  - `scroll_list`: mouse wheel scrolls the left list viewport without changing the selected task
  - `move_selection`: mouse wheel moves the selected row directly
- `task_sort_mode`
  - `due`: open tasks first, then completed tasks, ordered by due date
  - `priority`: open tasks first, then completed tasks, ordered by priority
  - `title`: open tasks first, then completed tasks, ordered by title
  - `created`: open tasks first, then completed tasks, ordered by newest task

You can change settings in either of these ways:

1. In-app: press `s`, use `up` / `down` or `tab` / `shift+tab`, then press `enter` or `ctrl+s`
2. Manually: inspect or edit the active project's `config` table in `.promag/projects/*.sqlite3`; manual edits bypass version checks

## Controls

### Views

- `1` `2` `3` `4` `5`: switch to Tasks, Team, Timeline, Archive, Help
- `tab` / `shift+tab`: cycle views
- `h` / `l`: previous / next view

### Navigation

- `j` / `k`, arrow keys: move through the active list
- `gg` / `G`: jump to first / last row
- `mouse wheel`: scroll the active pane
- `left click`: select a tab or row
- `M`: toggle app mouse capture vs terminal text selection
- `o`: cycle task sort mode

### Actions

- `:`: open action palette
- `a`: context-aware add action
- `m`: open member form
- `t`: open task form
- `e`: edit selected task or member
- `n`: open batch note capture
- `p`: open project switcher / create a project
- `f` or `/`: open filters
- `F`: clear all filters
- `s`: open settings
- `?`: open help
- `q`: quit

### Task Lifecycle

- `space`: toggle done in Task view
- `z`: archive completed task in Task view
- `r`: restore task in Archive view
- `x`: delete selected task, archived task, or empty member

### Forms

- `tab`: accept autocomplete when available, otherwise move forward
- `up` / `down`, `ctrl+j` / `ctrl+k`: move between fields
- `enter`: save task or apply filters
- `ctrl+s`: save task, member, note, filters, or settings
- `esc`: cancel the active modal

## Quick Note Capture

The note modal is designed for batch entry. You can set defaults once, then write task lines underneath them.

Defaults:

- `@Ali,Sara` sets assignees
- `#backend` sets category
- `!high` sets priority
- `due:next friday` sets due date
- `tags:api,release` sets tags

Task lines:

- Start with `-` for readability
- Inline tokens override the active defaults
- `// comment` stores a task comment

Example:

```text
@Ali,Sara
#backend
!high
due:next friday
tags:api,release

- Fix token refresh flow // validate mobile behavior
- Review deploy checklist @Ali #ops !urgent
```

New task and quick note forms default the due date to 7 days from today. Task and filter forms also accept natural-language dates such as `tomorrow`, `next friday`, `in 3 days`, and `Mar 20`.

## Data Files

- `.promag/registry.sqlite3`
  - Stores project metadata and the last-opened project
- `.promag/projects/*.sqlite3`
  - Stores members, tasks, and UI settings for each project
- `.promag/projects/*.sqlite3.bak-*`
  - Automatic timestamped backups created before older project databases are migrated
- Exported project JSON files
  - Store project metadata, members, tasks, settings, collaborators, and activity for backup or restore into a new local project
- Legacy `promag.sqlite3` / `promag-data.json` / `promag-config.json`
  - Imported automatically into a default project if they still exist on first run

Due dates are stored as `YYYY-MM-DD`. New tasks default to a due date 7 days from the creation date unless you change or clear the due date field.

## Collaboration Server

Run a local collaboration server for a project:

```bash
PROMAG_SERVER_TOKEN="$(openssl rand -hex 24)" go run . --serve Ops --addr :8080
```

Clients must send either:

```text
Authorization: Bearer <token>
```

or:

```text
X-ProMag-Token: <token>
```

Main API routes:

- `GET /state`: project metadata, config, tasks, members, collaborators, and activity log
- `GET /activity`: activity log only
- `GET /export`: project JSON export bundle
- `POST /tasks`, `PATCH /tasks/{id}`, `DELETE /tasks/{id}`
- `PATCH /tasks/{id}/status`, `PATCH /tasks/{id}/archive`
- `POST /members`, `PATCH /members/{id}`, `DELETE /members/{id}`
- `PATCH /config`

Write requests use optimistic versions. Send the version last seen as `expected_version`; stale writes return HTTP `409 Conflict`.

To connect from another ProMag TUI instance, the easiest path is to paste a
connection string (printed by the cloud commands below). For a plain `--serve`
project you can also configure it by hand:

1. Press `p`, create a project, and set its type to `remote`
2. Set the remote URL to the server, for example `http://localhost:8080`
3. Set the token in the form's Token field (or start the TUI with `PROMAG_REMOTE_TOKEN=<token>`)

The token is saved with the project, so you only enter it once.

Remote projects load and write through the server API. A local cache database is still kept under `.promag/projects/` so the project appears in the project switcher and can reload server state when opened. While a remote project is active, ProMag refreshes server state every few seconds, updates collaborator `last_seen_at` through authenticated requests, and shows active collaborators plus recent activity in the detail pane.

## Cloud Hub

Run a multi-project cloud hub when several clients need to connect to shared projects through one hosted server:

```bash
go run . --cloud-init --public-url https://promag.example.com   # writes ./data/promag.env once
go run . --cloud --addr :8080
```

Create or manage projects in the data directory (defaults to `./data`):

```bash
go run . --cloud-admin                       # interactive: projects + per-user tokens
go run . --cloud-create Ops
go run . --cloud-import Ops backups/ops.json
go run . --cloud-token Ops --token-label "manager laptop"
go run . --cloud-tokens Ops
go run . --cloud-revoke-token tok_example
go run . --cloud-backup backups/cloud.json
go run . --cloud-restore backups/cloud.json --data-dir ./restored
```

For managing several projects or onboarding many users, run the interactive
admin TUI with `--cloud-admin` (see the Self-Hosting Guide, Step 7). It browses
projects, creates them, and mints per-user tokens — pasting a whole list of
usernames at once — without typing a command per token.

`--cloud-create`, `--cloud-import`, and `--cloud-token` print a project token, a token ID, and a **connection string**. Share the connection string with one teammate; they paste it into ProMag and the remote URL and token are filled in automatically (no `PROMAG_REMOTE_TOKEN` needed). Pass `--public-url https://your-host` so the connection string points at your real, externally reachable address instead of `localhost`. Create one labeled token per person or device, list them with `--cloud-tokens`, and revoke a specific token ID with `--cloud-revoke-token`. The hub token from `--token` or `PROMAG_SERVER_TOKEN` remains the admin token for listing, creating, importing, backing up, restoring, and managing project tokens.

The running cloud hub also exposes admin-only token management routes under `/projects/{id}/tokens`, so hosted deployments can create, list, and revoke per-project tokens through HTTP without direct filesystem access. Token list responses omit token hashes, and newly created raw tokens are only returned once.

`--cloud-backup` writes one JSON file containing every cloud project's metadata, config, state, collaborators, activity, and project token hashes. Raw project tokens are not stored. `--cloud-restore` preserves project IDs and token hashes, including active and revoked token records, so existing remote URLs and project tokens continue to work, but it only restores into an empty cloud registry.

The HTTP servers write one access log line per external request to stdout. Project token creation and revocation are also written to the project's activity log.

Cloud API routes are project-scoped:

- `GET /projects`: list cloud projects; admin token required
- `POST /projects`: create a project with `{"name":"Ops"}`; admin token required
- `POST /projects/import`: import a JSON export bundle; admin token required
- `GET /projects/{id}`: project metadata; admin or project token required
- `GET /projects/{id}/tokens`: list project token metadata; admin token required
- `POST /projects/{id}/tokens`: create a project token with `{"label":"manager laptop"}`; admin token required
- `DELETE /projects/{id}/tokens/{token_id}`: revoke a project token; admin token required
- `GET /projects/{id}/state`: project state, config, collaborators, and activity; admin or project token required
- `GET /projects/{id}/export`: project JSON export bundle; admin or project token required
- `POST /projects/{id}/tasks`, `PATCH /projects/{id}/tasks/{task_id}`, `DELETE /projects/{id}/tasks/{task_id}`; admin or project token required
- `PATCH /projects/{id}/tasks/{task_id}/status`, `PATCH /projects/{id}/tasks/{task_id}/archive`; admin or project token required
- `POST /projects/{id}/members`, `PATCH /projects/{id}/members/{member_id}`, `DELETE /projects/{id}/members/{member_id}`; admin or project token required
- `PATCH /projects/{id}/config`; admin or project token required

To connect a TUI client to a cloud project, press `p` to open the project
switcher, create a project, and paste the connection string into the Name
field — ProMag fills in the rest and saves the token locally. To configure it by
hand instead, set the project type to `remote`, set the remote URL to
`http://localhost:8080/projects/{id}` (use the deployed host name when the hub is
on a server), and put the project token in the form's Token field.

### Docker Deployment

The repository includes a `Dockerfile`, `docker-compose.yml`, and `.env.example`
for running the cloud hub as a service.

Set your config once, then start the hub:

```bash
cp .env.example .env
# edit .env: set PROMAG_SERVER_TOKEN (e.g. openssl rand -hex 24) and PROMAG_PUBLIC_URL
docker compose up -d
```

Compose reads `.env`, injects the token and public URL into the container, and
stores all project data in the named volume `promag-cloud-data` (mounted at
`./data` inside the container — the default data directory, so no command ever
passes `--data-dir`). Do not run the hub without the volume, or projects are lost
when the container is replaced.

When you need to add projects or users, run the admin TUI through the same
service:

```bash
docker compose run --rm -it promag-cloud --cloud-admin
```

Or run individual one-off commands (no `--data-dir`, no token to pass — both come
from the volume and `.env`):

```bash
docker compose run --rm promag-cloud --cloud-create Ops
docker compose run --rm -v "$PWD/backups:/imports:ro" promag-cloud --cloud-import Ops /imports/ops.json
docker compose run --rm promag-cloud --cloud-token Ops --token-label "manager laptop"
docker compose run --rm promag-cloud --cloud-tokens Ops
docker compose run --rm promag-cloud --cloud-revoke-token tok_example
docker compose run --rm -v "$PWD/backups:/backups" promag-cloud --cloud-backup /backups/cloud.json
```

Pass `--public-url https://<host>` to `--cloud-create`/`--cloud-token` so the
printed connection string points at your real host. Then a teammate just runs
`promag`, presses `p`, and pastes the connection string — no environment
variables to set.

## Self-Hosting Guide (Start to Finish)

This walkthrough takes you from nothing to a running, team-ready ProMag cloud
hub that several people can connect to from their own terminals. It uses the
Docker deployment, which is the recommended way to self-host. A no-Docker
alternative is covered at the end.

### What you are building

- One **cloud hub** process (`--cloud`) that serves every project over an
  HTTP API and stores all data in a single data directory (a Docker volume).
- One **admin token**, stored in `promag.env` inside the data directory, that
  manages the hub: listing, creating, importing, backing up, restoring projects,
  and minting per-project tokens.
- One or more **project tokens**, one per person or device, that each grant
  access to a single project. You hand each one to a teammate as a single
  **connection string** they paste into ProMag.
- Each teammate runs the normal ProMag TUI and pastes the connection string to
  add a `remote` project that points at the hub.

```
 teammate TUI ──paste connection string──▶ reverse proxy (HTTPS) ──▶ cloud hub ──▶ /data volume
 teammate TUI ──paste connection string──▶        :443                  :8080        (SQLite)
```

### Prerequisites

- A Linux server (or any host) with Docker and the Docker Compose plugin.
- A DNS name pointing at the server if you want HTTPS (strongly recommended).
- `openssl` for generating tokens, or any source of random hex.
- For the no-Docker route only: Go 1.26+.

### Step 1 — Get the code

```bash
git clone <your-fork-or-this-repo-url> promag
cd promag
```

The repository already contains the `Dockerfile` and `docker-compose.yml` used
below.

### Step 2 — Set your config (`.env`)

Configuration lives in a `.env` file next to `docker-compose.yml`. Copy the
example and fill in your own values:

```bash
cp .env.example .env
```

Edit `.env`:

```ini
# the master credential — generate one with: openssl rand -hex 24
PROMAG_SERVER_TOKEN=paste-a-long-random-token-here
# the address teammates reach the hub at (your HTTPS host from Step 5)
PROMAG_PUBLIC_URL=https://promag.example.com
```

Keep `PROMAG_SERVER_TOKEN` safe — it is the admin master credential. `.env` is
git-ignored. (While testing you can set `PROMAG_PUBLIC_URL=http://<server-ip>:8080`
and change it later.)

### Step 3 — Start the hub

```bash
docker compose up -d
```

This starts the `promag-cloud` service on `:8080`. Compose reads `.env` and
injects your token and public URL; data is persisted in the named volume
`promag-cloud-data`, mounted at `./data` inside the container (the default data
directory — that's why no command needs `--data-dir`). The volume is what keeps
your projects across restarts and image rebuilds — never run the hub without it.

Verify it is healthy (the `/health` route needs no authentication):

```bash
curl -fsS http://localhost:8080/health
# {"status":"ok"}
```

Check logs and access lines (one line per external request) with:

```bash
docker compose logs -f promag-cloud
```

### Step 4 — Create your first project and token

You can create a project from the command line with `docker compose run` (the
admin TUI in Step 7 is the friendlier way for many projects/users). The data
directory and public URL come from the volume and `.env`, so you don't repeat
them:

```bash
docker compose run --rm promag-cloud --cloud-create Ops
```

This prints the project ID, remote URL, token ID, project token, and a
**connection string**. The token and connection string are shown **only once** —
copy the connection string immediately; it is the single thing you hand to a
teammate. Example output:

```
Created cloud project "Ops" (prj_...)
Remote URL: https://promag.example.com/projects/prj_...
Project token ID: tok_...
Project token: 6f1c... (48 hex chars)

Connection string (share with one teammate; they paste it into ProMag):
promag://join/eyJ2IjoxLCJ1cmwiOiJodHRwczovL3Byb21hZy5leGFtcGxl...
```

To seed a project from an existing JSON export instead of an empty one:

```bash
docker compose run --rm -v "$PWD/backups:/imports:ro" \
  promag-cloud --cloud-import Ops /imports/ops.json
```

### Step 5 — Put the hub behind HTTPS (important)

The hub speaks plain HTTP and authenticates with bearer tokens. If you expose
port 8080 directly over the internet, those tokens travel in cleartext. For any
deployment beyond `localhost`, terminate TLS in front of it with a reverse
proxy and only expose the proxy.

A minimal [Caddy](https://caddyserver.com) config (automatic Let's Encrypt
certificates) looks like this:

```caddyfile
promag.example.com {
    reverse_proxy localhost:8080
}
```

With a proxy in place, do not publish `8080` to the public internet — bind it to
localhost (or the Docker network) and let the proxy reach it. Your clients then
use `https://promag.example.com` as the host.

### Step 6 — Connect a client

On each teammate's machine, install or build ProMag (see **Build And Install**),
then run it:

```bash
promag
```

Inside the TUI:

1. Press `p` to open the project switcher, then `n` to create a new project.
2. Paste the connection string into the **Name** field.
3. Press `ctrl+s` to save.

That's it — ProMag recognizes the connection string, creates a `remote` project,
fills in the URL, stores the token locally (so no environment variable is
needed), keeps a local cache under `.promag/projects/`, refreshes every few
seconds, and shows active collaborators and recent activity.

To label who is connecting in the collaborator list and activity log, set
`PROMAG_REMOTE_ACTOR` before launching (optional but recommended):

```bash
PROMAG_REMOTE_ACTOR="ali@laptop" promag
```

> **Manual alternative.** If you prefer not to use a connection string, create
> the project, set its type to `remote`, set the remote URL to
> `https://promag.example.com/projects/<project-id>`, and paste the raw token
> into the form's **Token** field. The env var `PROMAG_REMOTE_TOKEN` still works
> as an override for advanced setups.

### Step 7 — Add teammates (admin TUI — recommended)

When you have several projects or many users, managing tokens with one long
command each gets tedious. The **admin TUI** does it interactively — browse
projects, create projects, and mint a per-user token (each with its own
connection string) for any of them:

```bash
docker compose run --rm -it promag-cloud --cloud-admin
```

(The `-it` flags give the container a terminal. For the binary, just run
`./promag --cloud-admin --data-dir /var/lib/promag`. The public URL comes from
`promag.env`.)

Inside the admin TUI:

- **Projects screen** — `↑/↓` select, `enter` to manage, `n` to create a new
  project, `q` to quit.
- **Manage screen** — see the project's tokens; press `a` to add users.
- **Add users** — paste a whole list (one username per line, or comma-separated)
  and press `ctrl+s`. One token and one connection string are created per name.
- **Connection strings** — press `e` to export them to a `invites-<project>.txt`
  file (mode `600`). Every string created in the session is also printed to your
  terminal when you quit, so nothing is lost.
- **Revoke** — on the manage screen, select a token and press `r`.

Hand each user their connection string; they connect exactly as in Step 6
(`promag` → `p` → `n` → paste → save). This is the whole "add 100 users across
projects" workflow: one screen for you, one paste for each of them.

> **Shared-token shortcut.** A token is *not* tied to one person — multiple
> people can use the same connection string at once and still appear as distinct
> collaborators (identity comes from `PROMAG_REMOTE_ACTOR` or their username). So
> if you don't need to revoke individuals, create **one** token and share its
> connection string with everyone. Per-user tokens only matter when you need to
> cut off one person without rotating everyone.

#### One-off CLI alternative

To mint a single token without the TUI (prints one connection string):

```bash
docker compose run --rm promag-cloud \
  --cloud-token Ops --token-label "ali laptop"
```

List and revoke tokens as people come and go:

```bash
docker compose run --rm promag-cloud --cloud-tokens Ops
docker compose run --rm promag-cloud --cloud-revoke-token tok_...
```

A running hub can also manage tokens over HTTP with the admin token, without
shell access to the server:

```bash
# list project tokens
curl -fsS -H "Authorization: Bearer $PROMAG_SERVER_TOKEN" \
  https://promag.example.com/projects/<project-id>/tokens

# create a new labeled token (raw token returned once)
curl -fsS -X POST -H "Authorization: Bearer $PROMAG_SERVER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"label":"sara phone"}' \
  https://promag.example.com/projects/<project-id>/tokens

# revoke one
curl -fsS -X DELETE -H "Authorization: Bearer $PROMAG_SERVER_TOKEN" \
  https://promag.example.com/projects/<project-id>/tokens/tok_...
```

### Step 8 — Back up and restore

Back up every project (metadata, config, state, collaborators, activity, and
project token hashes — raw tokens are never stored) to a single JSON file:

```bash
docker compose run --rm -v "$PWD/backups:/backups" \
  promag-cloud --cloud-backup /backups/cloud.json
```

Restore preserves project IDs and token hashes, so existing remote URLs and
tokens keep working — but it only restores into an **empty** data directory:

```bash
docker compose run --rm -v "$PWD/backups:/backups:ro" \
  promag-cloud --cloud-restore /backups/cloud.json
```

Schedule the backup command (for example via `cron`) and copy the resulting
JSON off the server.

### Step 9 — Update the deployment

```bash
git pull
docker compose up --build -d
```

The data volume is untouched by rebuilds. SQLite runs in WAL mode, so you will
see `*.sqlite3-wal` and `*.sqlite3-shm` sidecar files inside the volume
alongside each database — leave them in place; they are part of the database.

### No-Docker alternative

You can run the same hub directly from a built binary. Pick a data directory and
run it (ideally behind the same reverse proxy, and under a process manager such
as systemd). The config file lives in that directory, so the token and public
URL are set once:

```bash
go build -o promag .

# one-time setup: writes /var/lib/promag/promag.env with a generated token
./promag --cloud-init --data-dir /var/lib/promag --public-url https://promag.example.com
./promag --cloud-create --data-dir /var/lib/promag Ops
./promag --cloud-token Ops --token-label "ali laptop" --data-dir /var/lib/promag

# run the hub (reads token + public URL from the config file)
./promag --cloud --addr :8080 --data-dir /var/lib/promag
```

All the `--cloud-*` commands work the same way; just swap
`docker compose run --rm promag-cloud` for `./promag` and pass your
`--data-dir`. (In Docker the data directory defaults to `./data` in the volume,
so you never pass it there.)

### Troubleshooting

- **`cloud token is required`** — no token is configured. In Docker, set
  `PROMAG_SERVER_TOKEN` in `.env`. For the binary, run `--cloud-init` first or set
  `PROMAG_SERVER_TOKEN` / `--token`.
- **`set PROMAG_SERVER_TOKEN in .env`** (from `docker compose`) — you have not
  created `.env` yet. `cp .env.example .env` and set a token.
- **`401 unauthorized` from a client** — the project token is wrong or revoked,
  or the remote URL points at the wrong project ID. Re-mint a token with
  `--cloud-token` and paste the fresh connection string.
- **`remote token is required`** — the project has no stored token and no
  `PROMAG_REMOTE_TOKEN` is set. Re-create the project by pasting the connection
  string, or paste the token into the form's Token field.
- **`404` only with the admin token** — the project ID does not exist. Without
  the admin token, unknown and unauthorized projects both return `401` so IDs
  cannot be probed.
- **Client cannot reach the hub** — confirm `curl https://<host>/health` returns
  `{"status":"ok"}` from the client's network, and that your firewall exposes the
  proxy port (443), not 8080 directly.
- **Data disappeared after an update** — the hub ran without the persistent
  volume. Keep the named Docker volume (or, for the binary, always point
  `--data-dir` at the same directory).

## Developer Workflow

Useful commands while working on the app:

```bash
go test ./...
gofmt -w main.go
go mod tidy
```

Recommended routine:

1. Make the change
2. Run `gofmt -w` on touched Go files
3. Run `go test ./...`
4. Run `go mod tidy` if dependencies changed

## Project Notes

- The app uses Bubble Tea for runtime/event handling and Lip Gloss for styling
- The UI is full-screen and runs in the alternate screen buffer
- Mouse interaction depends on terminal support for cell motion events
- Projects are local by default; `remote` projects sync through a collaboration server or cloud hub (see **Self-Hosting Guide**)

## References

The README structure and workflow guidance here follows current Go and Bubble Tea documentation:

- Go `build` / `install`: https://go.dev/doc/tutorial/compile-install
- Go modules and dependency workflow: https://go.dev/doc/modules/managing-dependencies
- Go standard `flag` package: https://pkg.go.dev/flag
- Bubble Tea project docs: https://github.com/charmbracelet/bubbletea
