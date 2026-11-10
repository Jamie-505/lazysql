---
# lazysql-dd1s
title: Table definition (DDL) and server information
status: completed
type: feature
priority: normal
parent: lazysql-vq7e
created_at: 2026-11-10T17:54:41Z
updated_at: 2026-11-10T17:54:41Z
---

Show a table's CREATE statement and basic facts about the connected server.

- [x] `drivers.TableDDL` for MySQL, SQLite, ClickHouse (native) and PostgreSQL, SQL Server (rebuilt from the catalog)
- [x] `drivers.ServerInfo` for all five engines
- [x] Shared read-only `TextModal` with copy
- [x] `ShowTableDDL` (`D` on a table) and `ServerInfo` (`I`) commands
- [x] README

## Summary of Changes

- Shown in a modal, not as a sixth table menu tab as first planned: the menu surface and its metadata cache are tabular, and a text page there would have been a much larger change.
- Table and schema names are quoted as identifiers or string literals before they are placed in catalog queries.
- The SQL Server version lists columns and the primary key only and says so in its header.
- Tests: SQLite end to end, MySQL and PostgreSQL through sqlmock, SQL Server type rendering, server info. Live engines are checked manually by the user.
