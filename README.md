# ProMag

Simple project management TUI in Go for small teams. It uses Bubble Tea for a keyboard-first, mouse-friendly CLI.

## Features

- Team member management
- Task tracking per member
- Task metadata: category, priority, tags, comments, due date
- Three working views: task-based, member-based, and due-date-based
- Subtle color coding for status, priority, and member identity
- Quick note capture to create many tasks quickly
- Mouse support and `nvim`-style navigation
- Local JSON persistence in `promag-data.json`

## Run

```bash
go run .
```

## Controls

- `1` `2` `3` `4`: switch to Tasks, Members, Due Dates, Help
- `tab` / `shift+tab`: cycle views
- `h` `l`: previous / next view
- `j` `k`: move selection
- `gg` / `G`: first / last item
- `m`: add member
- `t`: add task
- `a`: context-aware add
- `n`: quick note capture
- `f` or `/`: open filters
- `F`: clear filters
- `space`: toggle done in Task View
- `x`: delete selected task or empty member
- `?`: open manual
- `q`: quit

## Quick Note Capture

The note view is built for fast entry. You can set defaults once, then write task lines underneath them.

```text
@Ali,Sara
#backend
!high
due:2026-03-20
tags:api,release

- Fix token refresh flow // validate mobile behavior
- Review deploy checklist @Ali #ops !urgent due:2026-03-18
```

Rules:

- `@name1,name2` sets assignees
- `#category` sets category
- `!priority` sets priority
- `due:YYYY-MM-DD` sets due date
- `tags:a,b,c` sets tags
- `// comment` stores a task comment
- Inline tokens on a task line override the active defaults

## Data

- All data is saved to `promag-data.json` in the current project directory.
- Due dates use `YYYY-MM-DD`.
- In the task form, the `Members` field accepts comma-separated names and creates one task per member.
