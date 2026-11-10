---
# lazysql-pn4k
title: Pane keys and input mode indicator
status: completed
type: feature
priority: normal
parent: lazysql-vq7e
created_at: 2026-11-10T21:40:00Z
updated_at: 2026-11-10T21:40:00Z
---

It was not visible whether keys were typed as text or acted as commands, nor which key focuses which pane.

- [x] Pane Key in the border title: tree (`H`), pane with the tabs (`L`), WHERE filter (`/`)
- [x] Input Mode on the bottom border of the focused pane: NORMAL, INSERT, VISUAL
- [x] README, CONTEXT.md

## Summary of Changes

- `components/pane_hints.go`: titles are built from the keymap, so rebound keys show correctly.
- `Home.inputMode` reads the same state the home key handler uses to decide whether navigation keys apply.
- The mode is read from the primitives' own focus state, not `Application.GetFocus`, which deadlocks while drawing.
- The SQL editor key (`Ctrl-E`) is not shown on a border: it was on the right pane title at first and was removed as clutter.
