---
# lazysql-tp1k
title: Database type picker for new connections
status: completed
type: feature
priority: normal
parent: lazysql-vq7e
created_at: 2026-11-10T18:33:34Z
updated_at: 2026-11-10T18:33:34Z
---

Adding a connection meant typing a full URL. Offer a type picker and a details form with common options such as SSL, like vi-sql.

- [x] Type picker: PostgreSQL, MySQL / MariaDB, SQL Server, ClickHouse, SQLite, Other (URL)
- [x] Details form per type: host, port (default filled), user, password, database, encryption option
- [x] Builds the connection URL and hands it to the existing form for review, test and save
- [x] README

## Summary of Changes

- `components/connection_builder.go`: `buildConnectionURL`, `ConnectionTypePicker`, `ConnectionBuilder`.
- The existing URL form stays the single place that saves, so keyring storage, read-only and "show all databases" work unchanged. Editing a connection still opens that form directly.
- Encryption defaults to off for every type; PostgreSQL always writes `sslmode` because the driver's own default is `require`.
- Tests check that every type and every encryption value produces a URL that parses to the right driver.
