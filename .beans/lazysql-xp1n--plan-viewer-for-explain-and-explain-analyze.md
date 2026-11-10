---
# lazysql-xp1n
title: Plan Viewer for EXPLAIN and EXPLAIN ANALYZE
status: completed
type: feature
priority: normal
parent: lazysql-vq7e
created_at: 2026-11-10T17:26:14Z
updated_at: 2026-11-10T17:26:14Z
---

Show the plan of an editor query in a modal, with an ANALYZE toggle.

- [x] `drivers.ExplainQuery` for PostgreSQL, MySQL (with MariaDB fallback), SQLite, SQL Server, ClickHouse
- [x] Honors the editor's target database through `Driver.ExecuteQuery`
- [x] PostgreSQL JSON plan parsed into a tree (`internal/explain`, adapted from vi-sql, Apache-2.0)
- [x] Plan Viewer modal: tree or text, `a` ANALYZE, `y` copy, `q`/Esc close
- [x] ANALYZE refused for anything but read-only SELECT statements
- [x] `ExplainQuery` command, `Ctrl-X` in the editor
- [x] README and CONTEXT.md

## Summary of Changes

- One `drivers.ExplainQuery` function switches on the provider instead of a per-driver interface: every engine but SQL Server only needs a statement prefix. SQL Server uses SHOWPLAN_TEXT on a reserved connection that is discarded afterwards.
- Opening the viewer always runs the plan-only form, also for a query typed as EXPLAIN ANALYZE.
- No telemetry event was added.
- Unit tests cover prefix stripping, each engine's statement (sqlmock / in-memory SQLite), plan parsing and modal rendering. Live engines are checked manually by the user.
