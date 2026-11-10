package drivers

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ExplainFormat tells the caller how to read ExplainResult.Text.
type ExplainFormat uint8

const (
	// ExplainFormatText is a plan already laid out as readable lines.
	ExplainFormatText ExplainFormat = iota
	// ExplainFormatPostgresJSON is the output of EXPLAIN (FORMAT JSON).
	ExplainFormatPostgresJSON
)

// ExplainResult is a query plan as returned by the database.
type ExplainResult struct {
	Format ExplainFormat
	Text   string
	// Analyzed reports that the query was executed to measure the plan.
	Analyzed bool
}

// SupportsExplainAnalyze reports whether the provider can execute a query and
// report the measured plan.
func SupportsExplainAnalyze(provider string) bool {
	return provider == DriverPostgres || provider == DriverMySQL
}

// StripExplainPrefix removes a leading EXPLAIN, EXPLAIN ANALYZE,
// EXPLAIN QUERY PLAN or EXPLAIN (...) and any trailing semicolon, so a query
// the user already wrote as an EXPLAIN is not explained twice. analyze
// reports whether the prefix asked for ANALYZE.
func StripExplainPrefix(query string) (bare string, analyze bool) {
	bare = strings.TrimRight(strings.TrimSpace(query), "; \t\r\n")
	if !hasKeywordPrefix(bare, "EXPLAIN") {
		return bare, false
	}
	bare = strings.TrimSpace(bare[len("EXPLAIN"):])
	if strings.HasPrefix(bare, "(") {
		if end := strings.Index(bare, ")"); end >= 0 {
			analyze = strings.Contains(strings.ToUpper(bare[:end]), "ANALYZE")
			bare = strings.TrimSpace(bare[end+1:])
		}
	}
	for _, keyword := range []string{"ANALYZE", "VERBOSE", "QUERY PLAN", "FORMAT=TREE", "FORMAT=JSON", "FORMAT=TRADITIONAL"} {
		if hasKeywordPrefix(bare, keyword) {
			if keyword == "ANALYZE" {
				analyze = true
			}
			bare = strings.TrimSpace(bare[len(keyword):])
		}
	}
	return bare, analyze
}

func hasKeywordPrefix(query, keyword string) bool {
	if len(query) < len(keyword) || !strings.EqualFold(query[:len(keyword)], keyword) {
		return false
	}
	if len(query) == len(keyword) {
		return true
	}
	next := query[len(keyword)]
	return next == ' ' || next == '\t' || next == '\n' || next == '\r' || next == '('
}

// ExplainQuery asks the database for the plan of query. With analyze the
// query is executed, so callers must only request it for statements that are
// safe to run. database selects the editor target, as for ExecuteQuery.
func ExplainQuery(ctx context.Context, db Driver, database, query string, analyze bool) (ExplainResult, error) {
	bare, _ := StripExplainPrefix(query)
	if bare == "" {
		return ExplainResult{}, errors.New("no query to explain")
	}
	provider := db.GetProvider()
	if analyze && !SupportsExplainAnalyze(provider) {
		return ExplainResult{}, fmt.Errorf("EXPLAIN ANALYZE is not supported for %s", provider)
	}

	switch provider {
	case DriverPostgres:
		statement := "EXPLAIN (FORMAT JSON) " + bare
		if analyze {
			statement = "EXPLAIN (ANALYZE, FORMAT JSON) " + bare
		}
		rows, _, err := db.ExecuteQuery(ctx, database, statement)
		if err != nil {
			return ExplainResult{}, err
		}
		return ExplainResult{Format: ExplainFormatPostgresJSON, Text: firstColumnText(rows), Analyzed: analyze}, nil

	case DriverMySQL:
		if analyze {
			rows, _, err := db.ExecuteQuery(ctx, database, "EXPLAIN ANALYZE "+bare)
			if err != nil {
				return ExplainResult{}, err
			}
			return ExplainResult{Text: firstColumnText(rows), Analyzed: true}, nil
		}
		rows, _, err := db.ExecuteQuery(ctx, database, "EXPLAIN FORMAT=TREE "+bare)
		if err == nil {
			return ExplainResult{Text: firstColumnText(rows)}, nil
		}
		if ctxErr := contextOrBackground(ctx).Err(); ctxErr != nil {
			return ExplainResult{}, err
		}
		// MariaDB and MySQL before 8.0.16 have no tree format.
		rows, _, fallbackErr := db.ExecuteQuery(ctx, database, "EXPLAIN "+bare)
		if fallbackErr != nil {
			return ExplainResult{}, fallbackErr
		}
		return ExplainResult{Text: tabularPlanText(rows)}, nil

	case DriverSqlite:
		rows, _, err := db.ExecuteQuery(ctx, database, "EXPLAIN QUERY PLAN "+bare)
		if err != nil {
			return ExplainResult{}, err
		}
		return ExplainResult{Text: sqliteQueryPlanText(rows)}, nil

	case DriverClickHouse:
		rows, _, err := db.ExecuteQuery(ctx, database, "EXPLAIN "+bare)
		if err != nil {
			return ExplainResult{}, err
		}
		return ExplainResult{Text: firstColumnText(rows)}, nil

	case DriverMSSQL:
		mssql, ok := db.(*MSSQL)
		if !ok {
			return ExplainResult{}, errors.New("query plans are not available for this SQL Server connection")
		}
		text, err := mssql.showplanText(ctx, database, bare)
		if err != nil {
			return ExplainResult{}, err
		}
		return ExplainResult{Text: text}, nil
	}

	return ExplainResult{}, fmt.Errorf("query plans are not supported for %s", provider)
}

// firstColumnText joins the first column of every data row. rows starts with
// the header row, as returned by Driver.ExecuteQuery.
func firstColumnText(rows [][]string) string {
	var lines []string
	for _, row := range dataRows(rows) {
		if len(row) > 0 {
			lines = append(lines, strings.TrimRight(row[0], "\n"))
		}
	}
	return strings.Join(lines, "\n")
}

func dataRows(rows [][]string) [][]string {
	if len(rows) <= 1 {
		return nil
	}
	return rows[1:]
}

// tabularPlanText lays a classic tabular EXPLAIN out as aligned columns.
func tabularPlanText(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, value := range row {
			if i < len(widths) && len([]rune(value)) > widths[i] {
				widths[i] = len([]rune(value))
			}
		}
	}
	var out strings.Builder
	for _, row := range rows {
		for i, value := range row {
			if i >= len(widths) {
				break
			}
			if i > 0 {
				out.WriteString("  ")
			}
			out.WriteString(value)
			if i < len(row)-1 {
				out.WriteString(strings.Repeat(" ", widths[i]-len([]rune(value))))
			}
		}
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n")
}

// sqliteQueryPlanText indents EXPLAIN QUERY PLAN rows (id, parent, notused,
// detail) by their depth in the plan tree.
func sqliteQueryPlanText(rows [][]string) string {
	depths := map[int]int{0: -1}
	var lines []string
	for _, row := range dataRows(rows) {
		if len(row) < 4 {
			if len(row) > 0 {
				lines = append(lines, row[len(row)-1])
			}
			continue
		}
		id, _ := strconv.Atoi(row[0])
		parent, _ := strconv.Atoi(row[1])
		depth := depths[parent] + 1
		depths[id] = depth
		lines = append(lines, strings.Repeat("  ", depth)+row[3])
	}
	return strings.Join(lines, "\n")
}

// showplanText returns the estimated plan of query without executing it.
// SHOWPLAN_TEXT is a session setting, so the statement runs on a reserved
// connection that is discarded afterwards instead of returned to the pool.
func (db *MSSQL) showplanText(ctx context.Context, database, query string) (string, error) {
	ctx = contextOrBackground(ctx)
	conn, err := db.Connection.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		_ = conn.Close()
	}()

	if database != "" && database != db.CurrentDatabase {
		if _, err := conn.ExecContext(ctx, "USE "+quoteMSSQLIdentifier(database)); err != nil {
			return "", err
		}
	}
	if _, err := conn.ExecContext(ctx, "SET SHOWPLAN_TEXT ON"); err != nil {
		return "", err
	}

	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	// The first result set repeats the statement, the following ones hold
	// the plan. Only the first column (StmtText) is text worth showing.
	var lines []string
	for {
		columns, err := rows.Columns()
		if err != nil {
			return "", err
		}
		for rows.Next() {
			values := make([]any, len(columns))
			targets := make([]any, len(columns))
			for i := range values {
				targets[i] = &values[i]
			}
			if err := rows.Scan(targets...); err != nil {
				return "", err
			}
			if len(values) == 0 {
				continue
			}
			switch value := values[0].(type) {
			case string:
				lines = append(lines, value)
			case []byte:
				lines = append(lines, string(value))
			}
		}
		if err := rows.Err(); err != nil {
			return "", err
		}
		if !rows.NextResultSet() {
			break
		}
	}
	return strings.Join(lines, "\n"), nil
}
