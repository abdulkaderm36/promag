# ProMag

ProMag is a Bubble Tea-based project management TUI written in Go. It is built for keyboard-first task tracking, but also supports mouse interaction, local SQLite persistence, archive workflows, and batch task capture.

## What It Does

- Manage team members and tasks in a terminal UI
- Switch between Tasks, Team, Timeline, Archive, and Help views
- Track task status, due dates, comments, tags, priority, and assignees
- Archive completed tasks without deleting them
- Capture multiple tasks quickly from plain-text notes
- Persist board state locally in a SQLite database file

## Requirements

- Go 1.26+
- A UTF-8 terminal with mouse support

## Quick Start

Run the app from the project root:

```bash
go run .
```

This creates and uses `promag.sqlite3` for tasks, members, and UI behavior settings.
If legacy `promag-data.json` or `promag-config.json` files are present, they are imported automatically on first run.

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

Examples:

```bash
go run . --debug
go run . --debug-hitboxes
go run . --debug --debug-hitboxes
```

## Configuration

Behavior settings are stored in `promag.sqlite3`.

Current settings:

- `left_wheel_mode`
  - `scroll_list`: mouse wheel scrolls the left list viewport without changing the selected task
  - `move_selection`: mouse wheel moves the selected row directly

You can change settings in either of these ways:

1. In-app: press `s`, use `up` / `down` or `tab` / `shift+tab`, then press `enter` or `ctrl+s`
2. Manually: inspect or edit the `config` table in `promag.sqlite3`

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

### Actions

- `:`: open action palette
- `a`: context-aware add action
- `m`: open member form
- `t`: open task form
- `e`: edit selected task or member
- `n`: open batch note capture
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

Task and filter forms also accept natural-language dates such as `tomorrow`, `next friday`, `in 3 days`, and `Mar 20`.

## Data Files

- `promag.sqlite3`
  - Stores members, tasks, and UI settings
- Legacy `promag-data.json` / `promag-config.json`
  - Imported automatically if they still exist when the app first opens the SQLite database

Due dates are stored as `YYYY-MM-DD`.

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
