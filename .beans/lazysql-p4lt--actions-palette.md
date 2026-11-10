---
# lazysql-p4lt
title: Actions Palette
status: completed
type: feature
priority: normal
parent: lazysql-vq7e
created_at: 2026-11-10T17:57:10Z
updated_at: 2026-11-10T17:57:10Z
---

A searchable list of the actions available for the focused surface.

- [x] Lists the keymap groups that apply to the focus (editor, tree, table, connection list) plus global actions
- [x] Fuzzy filter, arrow / Ctrl-N / Ctrl-P navigation, Enter runs, Esc closes
- [x] Running an entry replays its key, so every existing guard still applies
- [x] `ActionsPalette` global command on `Ctrl-O`
- [x] README and CONTEXT.md

## Summary of Changes

- `Ctrl-O` instead of the planned `Ctrl-K`: the editor uses `Ctrl-K` in insert mode.
- When a dialog or form field has the focus only global actions are listed; a replayed key would otherwise be typed into it.
- Each command and each key is listed once.
