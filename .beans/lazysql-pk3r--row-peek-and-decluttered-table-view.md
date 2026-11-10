---
# lazysql-pk3r
title: Row Peek and decluttered table view
status: completed
type: feature
priority: normal
parent: lazysql-vq7e
created_at: 2026-11-10T21:00:00Z
updated_at: 2026-11-10T21:00:00Z
---

The table view was crowded: the sidebar opened by itself and the table area sat in a box directly inside the right pane border. Offer a Row Peek like vi-sql instead.

- [x] Row Peek on `i`: every column of the row with name, type and value; full value of the selected column underneath
- [x] Copy value (`y`), copy name and value (`Y`), full screen (`f`)
- [x] Sidebar no longer opens by itself (`DisableSidebar` defaults to true, `S` still toggles)
- [x] Remove the box around menu, filter, grid and row count
- [x] README, CONTEXT.md

## Summary of Changes

- `components/row_peek.go`: `RowPeek`, opened from the table key handler. Own keymap group `rowpeek`, so it shows in help and the Actions Palette.
- `ResultsTable.rowFields` is shared by the sidebar and the Row Peek.
- `o`/`O` keep appending and duplicating rows; vi-sql's peek keys were not taken over. `I` is Server Info, so full screen is a key inside the peek.
- The read-only title moved from the removed box to the right pane border.
- An explicit `DisableSidebar = false` in an existing config keeps the sidebar opening by itself.
