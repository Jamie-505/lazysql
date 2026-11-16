package components

import (
	"strings"

	"github.com/jorgerojas26/lazysql/drivers"
	"github.com/jorgerojas26/lazysql/internal/sqlformat"
)

// sqlDialect returns the lexical rules the formatter needs for a provider.
func sqlDialect(provider string) sqlformat.Dialect {
	switch provider {
	case drivers.DriverPostgres:
		return sqlformat.Dialect{NestedComments: true, DollarQuotes: true}
	case drivers.DriverMySQL:
		return sqlformat.Dialect{BackslashEscapes: true, HashComments: true}
	case drivers.DriverMSSQL:
		return sqlformat.Dialect{NestedComments: true, BracketIdentifiers: true}
	case drivers.DriverSqlite:
		return sqlformat.Dialect{BracketIdentifiers: true}
	case drivers.DriverClickHouse:
		return sqlformat.Dialect{BackslashEscapes: true}
	}
	return sqlformat.Dialect{}
}

// queryReturnsRows classifies editor queries, not their safety. Read-only
// validation must still run independently before either execution path.
// A multi-statement script returns rows when any of its statements does, so
// "BEGIN; UPDATE ... RETURNING ..." shows the returned rows.
func queryReturnsRows(query, provider string) bool {
	for _, tokens := range sqlRoutingStatements(query, provider) {
		if statementReturnsRows(tokens) {
			return true
		}
	}
	return false
}

func statementReturnsRows(tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	switch tokens[0] {
	case "select", "with", "explain", "show", "describe", "desc", "values", "pragma":
		return true
	case "insert", "update", "delete":
		for _, token := range tokens[1:] {
			if token == "returning" {
				return true
			}
		}
	}
	return false
}

// sqlRoutingStatements splits the routing tokens on unquoted semicolons.
func sqlRoutingStatements(query, provider string) [][]string {
	var statements [][]string
	var current []string
	for _, token := range sqlRoutingLexemes(query, provider) {
		if token == ";" {
			statements = append(statements, current)
			current = nil
			continue
		}
		current = append(current, token)
	}
	return append(statements, current)
}

// sqlRoutingTokens returns unquoted words for result-set routing. Comments and
// quoted values/identifiers are skipped rather than stripped with regexes: a
// comment marker inside a string is data, and quoted RETURNING is not a clause.
// This is deliberately not a SQL parser or a read-only security check.
func sqlRoutingTokens(query, provider string) []string {
	var tokens []string
	for _, token := range sqlRoutingLexemes(query, provider) {
		if token != ";" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// sqlRoutingLexemes is sqlRoutingTokens plus ";" for each unquoted statement
// separator.
func sqlRoutingLexemes(query, provider string) []string {
	nestedComments := provider == drivers.DriverPostgres || provider == drivers.DriverMSSQL
	bracketIdentifiers := provider == drivers.DriverSqlite || provider == drivers.DriverMSSQL
	var tokens []string
	for i := 0; i < len(query); {
		switch {
		case strings.HasPrefix(query[i:], "--"):
			for i < len(query) && query[i] != '\n' && query[i] != '\r' {
				i++
			}
		case strings.HasPrefix(query[i:], "/*"):
			i += 2
			depth := 1
			for i < len(query) && depth > 0 {
				switch {
				case nestedComments && strings.HasPrefix(query[i:], "/*"):
					depth++
					i += 2
				case strings.HasPrefix(query[i:], "*/"):
					depth--
					i += 2
				default:
					i++
				}
			}
		case query[i] == '\'' || query[i] == '"' || query[i] == '`' || (bracketIdentifiers && query[i] == '['):
			quote := query[i]
			// MySQL/MariaDB strings use backslash escapes by default. PostgreSQL
			// enables them only for E'...' with standard_conforming_strings on;
			// SQLite never treats backslashes as string escapes.
			escaped := provider == drivers.DriverMySQL && (quote == '\'' || quote == '"')
			if provider == drivers.DriverPostgres && quote == '\'' && i > 0 &&
				(query[i-1] == 'e' || query[i-1] == 'E') &&
				(i == 1 || !sqlRoutingWordByte(query[i-2])) {
				escaped = true
			}
			if quote == '[' {
				quote = ']'
			}
			i++
			for i < len(query) {
				if escaped && query[i] == '\\' && i+1 < len(query) {
					i += 2
				} else if query[i] == quote {
					i++
					if i < len(query) && query[i] == quote {
						i++ // SQL doubles the delimiter to escape it.
						continue
					}
					break
				} else {
					i++
				}
			}
		case provider == drivers.DriverPostgres && query[i] == '$':
			// PostgreSQL dollar-quoted strings: $$...$$ or $tag$...$tag$.
			end := i + 1
			for end < len(query) && query[end] != '$' && sqlRoutingWordByte(query[end]) {
				end++
			}
			if end < len(query) && query[end] == '$' &&
				(end == i+1 || query[i+1] < '0' || query[i+1] > '9') {
				delimiter := query[i : end+1]
				i = end + 1
				if closeAt := strings.Index(query[i:], delimiter); closeAt >= 0 {
					i += closeAt + len(delimiter)
				} else {
					i = len(query)
				}
			} else {
				i++
			}
		case sqlRoutingWordByte(query[i]):
			start := i
			for i < len(query) && sqlRoutingWordByte(query[i]) {
				i++
			}
			tokens = append(tokens, strings.ToLower(query[start:i]))
		case query[i] == ';':
			tokens = append(tokens, ";")
			i++
		default:
			i++
		}
	}
	return tokens
}

func sqlRoutingWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c >= '0' && c <= '9' || c == '_' || c == '$' || c >= 0x80
}
