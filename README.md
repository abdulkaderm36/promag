# ProMag

Simple project management TUI in Go for small teams. It uses Bubble Tea for a keyboard-first, mouse-friendly CLI.

## Features

- Team member management
- Task tracking per member
- Task metadata: category, priority, tags, comments, due date
- Three working views: task-based, member-based, and due-date-based
- Subtle color coding for status, priority, and member identity
- Batch note capture with default member and due-date scope
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
- `tab`: accept a member autocomplete suggestion when available, otherwise move to next field
- `up` / `down`: move between fields while editing forms
- `ctrl+j` / `ctrl+k`: move between fields while editing forms
- `space`: toggle done in Task View
- `x`: delete selected task or empty member
- `?`: open manual
- `q`: quit

## Quick Note Capture

The note view is built for fast entry. You can set defaults once, then write task lines underneath them.

The batch entry modal also has dedicated top fields for:

- default member
- default due date

If you open it from Member View or Due Date View, those fields are prefilled from the current selection.

```text
@Ali,Sara
#backend
!high
due:next friday
tags:api,release

- Fix token refresh flow // validate mobile behavior
- Review deploy checklist @Ali #ops !urgent
```

Rules:

- `@name1,name2` sets assignees
- `#category` sets category
- `!priority` sets priority
- `due:YYYY-MM-DD` or `due:tomorrow` / `due:next friday` sets due date
- `tags:a,b,c` sets tags
- `// comment` stores a task comment
- Inline tokens on a task line override the active defaults

Task form and filter inputs also accept natural language dates like `tomorrow`, `next friday`, `in 3 days`, and `Mar 20`.

## Data

- All data is saved to `promag-data.json` in the current project directory.
- Due dates use `YYYY-MM-DD`.
- In the task form, the `Members` field accepts comma-separated names and creates one task per member.
