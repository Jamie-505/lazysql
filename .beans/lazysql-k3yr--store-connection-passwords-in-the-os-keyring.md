---
# lazysql-k3yr
title: Store connection passwords in the OS keyring
status: completed
type: feature
priority: normal
parent: lazysql-vq7e
created_at: 2026-11-10T17:21:44Z
updated_at: 2026-11-10T17:21:44Z
---

Connection passwords were written in plain text inside the URL in config.toml.

A `${keyring:name}` placeholder inside the URL was ruled out: Go net/url rejects braces in userinfo ("invalid userinfo") and the connection form parses the URL. A `Keyring` account field is used instead.

- [x] `internal/secrets`: Store interface, OS keyring implementation, memory store for tests
- [x] Split/inject the URL password without re-encoding the rest of the URL
- [x] Saving from the form moves the password to the keyring; plain-text fallback with a warning
- [x] Password resolved at connect and at "test connection", never at startup
- [x] Deleting a connection deletes its entry
- [x] Untouched `${env:...}` URLs keep their template
- [x] `StorePasswordsInKeyring` application setting
- [x] README and CONTEXT.md

## Summary of Changes

- New dependency github.com/zalando/go-keyring.
- `models.Connection.Keyring` holds a generated account id, so renames need no keyring move and local/global names cannot collide.
- `components/connection_secrets.go` holds the save, resolve and forget steps; the form, the connect flow and the delete flow call them.
- The resolved URL only lives in memory for the open connection (the external editor still receives it in LAZYSQL_CONNECTION_URL).
- No bulk migration action: editing and saving a connection moves its password. Field-based MSSQL connections (Username/Password keys) are unchanged.
- Tests use the memory store. The real keyring is only checked manually by the user.
