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
- `--cloud-create <project-name>`
  - Creates a project in the cloud data directory
- `--cloud-import <project-name> <path.json>`
  - Imports a project JSON file into the cloud data directory
- `--cloud-token <project-id-or-name>`
  - Creates and prints a cloud project access token
- `--token-label <label>`
  - Optional label for `--cloud-token`
- `--cloud-tokens <project-id-or-name>`
  - Lists cloud project access token IDs, labels, and status
- `--cloud-revoke-token <token-id>`
  - Revokes one cloud project access token by token ID
- `--cloud-backup <path.json>`
  - Backs up all cloud projects to a JSON file, including project token hashes
- `--cloud-restore <path.json>`
  - Restores all cloud projects from a JSON backup into an empty cloud data directory
- `--data-dir <path>`
  - Data directory for cloud commands; defaults to `.promag-cloud`
- `--addr <host:port>`
  - Address for `--serve` or `--cloud`; defaults to `:8080`
- `--token <token>`
  - Bearer token for `--serve` or `--cloud`; can also be set with `PROMAG_SERVER_TOKEN`

Remote project clients use `remote_url` plus `PROMAG_REMOTE_TOKEN` for authentication. Set `PROMAG_REMOTE_ACTOR` to control the collaborator ID sent with refreshes and writes.

Examples:

```bash
go run . --debug
go run . --debug-hitboxes
go run . --debug --debug-hitboxes
go run . --export Ops backups/ops.json
go run . --import "Restored Project" backups/ops.json
go run . --serve Ops --addr :8080 --token "$PROMAG_SERVER_TOKEN"
go run . --cloud-create --data-dir .promag-cloud Ops
go run . --cloud-import --data-dir .promag-cloud Ops backups/ops.json
go run . --cloud-token Ops --token-label "manager laptop" --data-dir .promag-cloud
go run . --cloud-tokens Ops --data-dir .promag-cloud
go run . --cloud-revoke-token tok_example --data-dir .promag-cloud
go run . --cloud-backup backups/cloud.json --data-dir .promag-cloud
go run . --cloud-restore backups/cloud.json --data-dir .promag-cloud
go run . --cloud --addr :8080 --token "$PROMAG_SERVER_TOKEN" --data-dir .promag-cloud
```

## Configuration

Behavior settings are stored inside each project's SQLite database.

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
2. Manually: inspect or edit the active project's `config` table in `.promag/projects/*.sqlite3`

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

To connect from another ProMag TUI instance:

1. Create or edit a project with type `remote`
2. Set the remote URL to the server, for example `http://localhost:8080`
3. Start the TUI with `PROMAG_REMOTE_TOKEN=<token>`

Remote projects load and write through the server API. A local cache database is still kept under `.promag/projects/` so the project appears in the project switcher and can reload server state when opened. While a remote project is active, ProMag refreshes server state every few seconds, updates collaborator `last_seen_at` through authenticated requests, and shows active collaborators plus recent activity in the detail pane.

## Cloud Hub

Run a multi-project cloud hub when several clients need to connect to shared projects through one hosted server:

```bash
PROMAG_SERVER_TOKEN="$(openssl rand -hex 24)" go run . --cloud --addr :8080 --data-dir .promag-cloud
```

Create or import projects into the cloud data directory:

```bash
go run . --cloud-create --data-dir .promag-cloud Ops
go run . --cloud-import --data-dir .promag-cloud Ops backups/ops.json
go run . --cloud-token Ops --token-label "manager laptop" --data-dir .promag-cloud
go run . --cloud-tokens Ops --data-dir .promag-cloud
go run . --cloud-revoke-token tok_example --data-dir .promag-cloud
go run . --cloud-backup backups/cloud.json --data-dir .promag-cloud
go run . --cloud-restore backups/cloud.json --data-dir .promag-cloud-restored
```

`--cloud-create`, `--cloud-import`, and `--cloud-token` print a project token and token ID. Use the printed token as `PROMAG_REMOTE_TOKEN` for clients that should only access that project. Create one labeled token per person or device, list them with `--cloud-tokens`, and revoke a specific token ID with `--cloud-revoke-token`. The hub token from `--token` or `PROMAG_SERVER_TOKEN` remains the admin token for listing, creating, and importing projects.

`--cloud-backup` writes one JSON file containing every cloud project's metadata, config, state, collaborators, activity, and project token hashes. Raw project tokens are not stored. `--cloud-restore` preserves project IDs and token hashes, including active and revoked token records, so existing remote URLs and project tokens continue to work, but it only restores into an empty cloud registry.

The HTTP servers write one access log line per external request to stdout. Project token creation and revocation are also written to the project's activity log.

Cloud API routes are project-scoped:

- `GET /projects`: list cloud projects; admin token required
- `POST /projects`: create a project with `{"name":"Ops"}`; admin token required
- `POST /projects/import`: import a JSON export bundle; admin token required
- `GET /projects/{id}`: project metadata; admin or project token required
- `GET /projects/{id}/state`: project state, config, collaborators, and activity; admin or project token required
- `GET /projects/{id}/export`: project JSON export bundle; admin or project token required
- `POST /projects/{id}/tasks`, `PATCH /projects/{id}/tasks/{task_id}`, `DELETE /projects/{id}/tasks/{task_id}`; admin or project token required
- `POST /projects/{id}/members`, `PATCH /projects/{id}/members/{member_id}`, `DELETE /projects/{id}/members/{member_id}`; admin or project token required
- `PATCH /projects/{id}/config`; admin or project token required

To connect a TUI client to a cloud project, create or edit a `remote` project and set its remote URL to `http://localhost:8080/projects/{id}`. Use the deployed host name instead of `localhost` when the hub is running on a server, and set `PROMAG_REMOTE_TOKEN` to that project's token.

### Docker Deployment

The repository includes a `Dockerfile` and `docker-compose.yml` for running the cloud hub as a service.

Build and start the hub:

```bash
export PROMAG_SERVER_TOKEN="$(openssl rand -hex 24)"
docker compose up --build -d
```

The compose file stores cloud project data in the named Docker volume `promag-cloud-data`, mounted at `/data` inside the container. Do not run the cloud hub without a persistent volume, or project databases will be lost when the container is replaced.

Create or import cloud projects by running one-off commands against the same volume:

```bash
docker compose run --rm promag-cloud --cloud-create --data-dir /data Ops
docker compose run --rm -v "$PWD/backups:/imports:ro" promag-cloud --cloud-import --data-dir /data Ops /imports/ops.json
docker compose run --rm promag-cloud --cloud-token Ops --token-label "manager laptop" --data-dir /data
docker compose run --rm promag-cloud --cloud-tokens Ops --data-dir /data
docker compose run --rm promag-cloud --cloud-revoke-token tok_example --data-dir /data
docker compose run --rm -v "$PWD/backups:/backups" promag-cloud --cloud-backup /backups/cloud.json --data-dir /data
```

For a remote client, use:

```bash
PROMAG_REMOTE_TOKEN="<project-token>" promag
```

Then create or edit a `remote` project and set the remote URL to `http://<host>:8080/projects/<project-id>`.

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
- Data is local-only; there is no remote sync layer

## References

The README structure and workflow guidance here follows current Go and Bubble Tea documentation:

- Go `build` / `install`: https://go.dev/doc/tutorial/compile-install
- Go modules and dependency workflow: https://go.dev/doc/modules/managing-dependencies
- Go standard `flag` package: https://pkg.go.dev/flag
- Bubble Tea project docs: https://github.com/charmbracelet/bubbletea
