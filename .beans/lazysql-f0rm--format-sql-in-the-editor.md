---
# lazysql-f0rm
title: Format SQL in the editor
status: completed
type: feature
priority: normal
parent: lazysql-vq7e
created_at: 2026-11-10T17:51:54Z
updated_at: 2026-11-10T17:51:54Z
---

Lay out the editor query one clause per line with a keybinding.

The plan was to port vi-sql's formatter. It changes the meaning of queries, so it was not ported: `:name` became `: name`, `N'uni'` became `N' uni '`, `a->>'k'` became `a - > > 'k'`, `0x1F` became `0 x1F`.

- [x] `internal/sqlformat`: dialect-aware tokenizer and whitespace-only formatter
- [x] Never inserts whitespace between tokens written glued together
- [x] Returns the input unchanged if the output would tokenize differently
- [x] `FormatQuery` command, `Ctrl-F` in the editor, one undo step
- [x] README

## Summary of Changes

- Clause keywords, JOINs, AND/OR in WHERE/HAVING/ON, subqueries and long SELECT lists get their own lines. Clause words inside function calls (`extract(year from d)`, `over (order by ...)`) stay inline.
- Dialect per provider decides string escapes, nested comments, bracket identifiers, dollar quotes and `#` comments.
- Keyword case is left as typed.
- Tests: layout goldens, idempotence, token preservation across parameter styles, operators, string prefixes and quoting.
