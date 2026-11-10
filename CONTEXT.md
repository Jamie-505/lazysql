# Lazysql

Context language for interactive database exploration in terminal UI.

## Language

**Foreign Key Jump**:
Keyboard action that follows a foreign key value from a source row cell to a referenced table view with a prefilled filter.
_Avoid_: FK drilldown, relation follow, link jump

## Relationships

- A **Foreign Key Jump** starts from one selected table cell in a source row.
- A **Foreign Key Jump** targets one referenced table view filtered by the selected value.

## Example dialogue

> **Dev:** "Should Enter open JSON or follow relation here?"
> **Domain expert:** "If this cell supports **Foreign Key Jump**, Enter follows relation; otherwise Enter keeps existing behavior."

## Flagged ambiguities

- "drilldown" was used to mean both JSON inspection and FK navigation - resolved: use **Foreign Key Jump** only for relation navigation.

## Loading state

**Non-blocking Loading**:
A state where a DB operation is in-flight but the UI remains interactive. A status line indicator replaces the previous blocking modal.
_Avoid_: blocking modal, loading overlay

**Loading Indicator**:
A thin text indicator on the table's pagination bar showing "Loading..." while a query runs. Does not steal focus or block input.
_Avoid_: loading spinner modal

**Load Cancellation**:
When a new load is triggered on a table with an in-flight load, the previous operation is cancelled via context cancellation before starting the new one.
_Avoid_: queue, race

**Stale Data**:
Old table results remain visible during a reload. Only cleared on success when new data arrives. Exception: SQL editor queries clear immediately on execute.
_Avoid_: blank screen during load

## Themes

**Transparent Theme**:
Any theme preset drawn on the terminal's own background instead of the preset's painted background. Set with `[theme] Transparent`.
_Avoid_: see-through mode, no-background preset

## Credentials

**Keyring Password**:
A connection password held in the operating system keyring under the connection's `Keyring` account. The config file stores the URL without it; it is added at connect time.
_Avoid_: encrypted password, secret placeholder

## Query plans

**Plan Viewer**:
Modal that shows the database's plan for one SQL editor query. Opening it never executes the query.
_Avoid_: explain window, explain tab

**Measured Plan**:
A plan produced by `EXPLAIN ANALYZE`, which executes the query. Only offered for read-only statements, and only from inside the **Plan Viewer**.
_Avoid_: real plan, analyze mode
