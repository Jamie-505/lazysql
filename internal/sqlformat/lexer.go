// Package sqlformat lays SQL out one clause per line.
//
// It never rewrites a token: only the whitespace between tokens changes, and
// whitespace is never inserted between two tokens the user wrote glued
// together (":name", "N'text'", "->>"), except where that is known to be safe.
package sqlformat

import "strings"

// Dialect holds the lexical differences between databases that decide where
// a string or comment ends.
type Dialect struct {
	// BackslashEscapes: a backslash escapes the next character inside
	// strings (MySQL, ClickHouse).
	BackslashEscapes bool
	// NestedComments: block comments nest (PostgreSQL, SQL Server).
	NestedComments bool
	// BracketIdentifiers: [name] quotes an identifier (SQL Server, SQLite).
	BracketIdentifiers bool
	// DollarQuotes: $$...$$ and $tag$...$tag$ are strings (PostgreSQL).
	DollarQuotes bool
	// HashComments: # starts a line comment and "--" needs a following
	// space to do so (MySQL).
	HashComments bool
}

type tokenKind uint8

const (
	kindWord tokenKind = iota
	kindString
	kindQuoted
	kindLineComment
	kindBlockComment
	kindPunct    // one of ( ) , ;
	kindOperator // a run of other symbol characters
)

type token struct {
	kind tokenKind
	text string
	// gap: whitespace separated this token from the previous one.
	gap bool
	// newlineBefore: that whitespace contained a line break.
	newlineBefore bool
}

func (t token) is(kind tokenKind, text string) bool {
	return t.kind == kind && t.text == text
}

func (t token) upper() string {
	if t.kind != kindWord {
		return ""
	}
	return strings.ToUpper(t.text)
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '_' || c == '$' || c >= 0x80
}

func isPunct(c byte) bool {
	return c == '(' || c == ')' || c == ',' || c == ';'
}

func startsLineComment(sql string, i int, d Dialect) bool {
	if d.HashComments && sql[i] == '#' {
		return true
	}
	if !strings.HasPrefix(sql[i:], "--") {
		return false
	}
	if !d.HashComments {
		return true
	}
	// In MySQL "a--1" is arithmetic; a comment needs "-- ".
	return i+2 >= len(sql) || isSpace(sql[i+2])
}

// quotedEnd returns the index just past the closing delimiter of the quoted
// run starting at start. A doubled delimiter is an escaped delimiter.
func quotedEnd(sql string, start int, closer byte, backslash bool) int {
	i := start + 1
	for i < len(sql) {
		switch {
		case backslash && sql[i] == '\\' && i+1 < len(sql):
			i += 2
		case sql[i] == closer:
			i++
			if i < len(sql) && sql[i] == closer {
				i++
				continue
			}
			return i
		default:
			i++
		}
	}
	return len(sql)
}

func blockCommentEnd(sql string, start int, nested bool) int {
	i := start + 2
	depth := 1
	for i < len(sql) {
		switch {
		case nested && strings.HasPrefix(sql[i:], "/*"):
			depth++
			i += 2
		case strings.HasPrefix(sql[i:], "*/"):
			depth--
			i += 2
			if depth == 0 {
				return i
			}
		default:
			i++
		}
	}
	return len(sql)
}

// dollarQuoteEnd returns the end of a PostgreSQL dollar-quoted string
// starting at start, or 0 when start does not open one.
func dollarQuoteEnd(sql string, start int) int {
	tagEnd := start + 1
	for tagEnd < len(sql) && sql[tagEnd] != '$' && isWordByte(sql[tagEnd]) {
		tagEnd++
	}
	if tagEnd >= len(sql) || sql[tagEnd] != '$' {
		return 0
	}
	if tagEnd > start+1 && sql[start+1] >= '0' && sql[start+1] <= '9' {
		return 0 // $1$ is a parameter followed by something else.
	}
	delimiter := sql[start : tagEnd+1]
	body := tagEnd + 1
	if closeAt := strings.Index(sql[body:], delimiter); closeAt >= 0 {
		return body + closeAt + len(delimiter)
	}
	return len(sql)
}

func tokenize(sql string, d Dialect) []token {
	var tokens []token
	gap, newline := false, false
	add := func(kind tokenKind, text string) {
		tokens = append(tokens, token{kind: kind, text: text, gap: gap, newlineBefore: newline})
		gap, newline = false, false
	}

	for i := 0; i < len(sql); {
		c := sql[i]
		switch {
		case isSpace(c):
			gap = true
			if c == '\n' {
				newline = true
			}
			i++
		case startsLineComment(sql, i, d):
			end := i
			for end < len(sql) && sql[end] != '\n' {
				end++
			}
			add(kindLineComment, strings.TrimRight(sql[i:end], " \t\r"))
			i = end
		case strings.HasPrefix(sql[i:], "/*"):
			end := blockCommentEnd(sql, i, d.NestedComments)
			add(kindBlockComment, sql[i:end])
			i = end
		case c == '\'':
			backslash := d.BackslashEscapes
			// PostgreSQL E'...' strings use backslash escapes.
			if n := len(tokens); !gap && n > 0 && tokens[n-1].kind == kindWord && strings.EqualFold(tokens[n-1].text, "e") {
				backslash = true
			}
			end := quotedEnd(sql, i, '\'', backslash)
			add(kindString, sql[i:end])
			i = end
		case c == '"':
			end := quotedEnd(sql, i, '"', d.BackslashEscapes)
			add(kindQuoted, sql[i:end])
			i = end
		case c == '`':
			end := quotedEnd(sql, i, '`', false)
			add(kindQuoted, sql[i:end])
			i = end
		case c == '[' && d.BracketIdentifiers:
			end := quotedEnd(sql, i, ']', false)
			add(kindQuoted, sql[i:end])
			i = end
		case c == '$' && d.DollarQuotes && dollarQuoteEnd(sql, i) > 0:
			end := dollarQuoteEnd(sql, i)
			add(kindString, sql[i:end])
			i = end
		case isWordByte(c):
			end := i
			for end < len(sql) && isWordByte(sql[end]) {
				if sql[end] == '$' && d.DollarQuotes && end > i && dollarQuoteEnd(sql, end) > 0 {
					break
				}
				end++
			}
			add(kindWord, sql[i:end])
			i = end
		case isPunct(c):
			add(kindPunct, sql[i:i+1])
			i++
		default:
			end := i + 1
			for end < len(sql) && isOperatorByte(sql, end, d) {
				end++
			}
			add(kindOperator, sql[i:end])
			i = end
		}
	}
	return tokens
}

// isOperatorByte reports whether the byte at i continues an operator run.
func isOperatorByte(sql string, i int, d Dialect) bool {
	c := sql[i]
	if isSpace(c) || isWordByte(c) || isPunct(c) || c == '\'' || c == '"' || c == '`' {
		return false
	}
	if c == '[' && d.BracketIdentifiers {
		return false
	}
	return !startsLineComment(sql, i, d) && !strings.HasPrefix(sql[i:], "/*")
}
