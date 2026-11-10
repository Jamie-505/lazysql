package sqlformat

import "strings"

const indentUnit = "  "

// A SELECT list is put one column per line when it is longer than this many
// characters, or has at least selectListBreakItems columns.
const (
	selectListBreakWidth = 60
	selectListBreakItems = 4
)

var joinModifiers = map[string]bool{
	"LEFT": true, "RIGHT": true, "INNER": true, "FULL": true,
	"CROSS": true, "NATURAL": true, "OUTER": true,
}

// spacedOperators are comparison operators that always read better with a
// space on both sides and cannot be part of a longer token.
var spacedOperators = map[string]bool{
	"=": true, "<>": true, "!=": true, "<=": true, ">=": true, "<": true, ">": true, "||": true,
}

var subqueryStarts = map[string]bool{
	"SELECT": true, "WITH": true, "INSERT": true, "UPDATE": true, "DELETE": true, "VALUES": true,
}

var selectListEnds = map[string]bool{
	"FROM": true, "WHERE": true, "GROUP": true, "ORDER": true, "HAVING": true, "LIMIT": true,
	"UNION": true, "EXCEPT": true, "INTERSECT": true, "INTO": true, "WINDOW": true,
}

// frame is one nesting level: the statement itself or a parenthesis.
type frame struct {
	// subquery: clause keywords inside start new lines. False for function
	// arguments, IN lists and the like, which stay on one line.
	subquery    bool
	base        int // indent of clause keywords
	closeIndent int // indent of the closing parenthesis
	started     bool
	first       string // first word of the statement
	clause      string
	between     int // BETWEEN ... AND pairs still waiting for their AND
	caseDepth   int
}

// Format returns sql laid out one clause per line. If the result would not
// consist of exactly the same tokens as the input, the input is returned
// unchanged.
func Format(sql string, d Dialect) string {
	tokens := tokenize(sql, d)
	if len(tokens) == 0 {
		return sql
	}
	formatted := render(tokens)
	if !sameTokens(tokens, tokenize(formatted, d)) {
		return sql
	}
	return formatted
}

func sameTokens(a, b []token) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].kind != b[i].kind || a[i].text != b[i].text {
			return false
		}
	}
	return true
}

func isComment(t token) bool {
	return t.kind == kindLineComment || t.kind == kindBlockComment
}

func endsOperand(t token) bool {
	return t.kind == kindWord || t.kind == kindString || t.kind == kindQuoted || t.is(kindPunct, ")")
}

func startsOperand(t token) bool {
	return t.kind == kindWord || t.kind == kindString || t.kind == kindQuoted || t.is(kindPunct, "(")
}

func render(tokens []token) string {
	var out strings.Builder
	frames := []frame{{subquery: true}}
	breakAfter := map[int]bool{}
	lineIndent := 0

	for i, tok := range tokens {
		top := &frames[len(frames)-1]
		closesSubquery, closeIndent := false, 0
		if tok.is(kindPunct, ")") && len(frames) > 1 {
			closesSubquery, closeIndent = top.subquery, top.closeIndent
			frames = frames[:len(frames)-1]
			top = &frames[len(frames)-1]
		}

		newlineIndent, blank, space := -1, false, false
		if i > 0 {
			prev := tokens[i-1]
			upper := tok.upper()
			switch {
			case prev.is(kindPunct, ";") && len(frames) == 1:
				blank, newlineIndent = true, 0
				*top = frame{subquery: true}
			case closesSubquery:
				newlineIndent = closeIndent
			case prev.is(kindPunct, "(") && top.subquery && !top.started && len(frames) > 1:
				newlineIndent = top.base
			case isComment(tok) && tok.newlineBefore:
				newlineIndent = top.base
			case top.subquery && top.started && tok.kind == kindWord &&
				(tok.gap || prev.is(kindPunct, ")")) && clauseStart(tokens, i, top) != "":
				newlineIndent = top.base
				top.clause = clauseStart(tokens, i, top)
				top.between = 0
				if top.clause == "SELECT" {
					markSelectList(tokens, i, breakAfter)
				}
			case top.subquery && tok.gap && (upper == "AND" || upper == "OR") && top.caseDepth == 0 &&
				(top.clause == "WHERE" || top.clause == "HAVING" || top.clause == "JOIN"):
				if upper == "AND" && top.between > 0 {
					top.between--
					space = true
				} else {
					newlineIndent = top.base + 1
				}
			case breakAfter[i-1]:
				newlineIndent = top.base + 1
			case tok.is(kindPunct, ","), tok.is(kindPunct, ";"), tok.is(kindPunct, ")"), prev.is(kindPunct, "("):
				// No space.
			case prev.is(kindPunct, ","):
				space = true
			case tok.kind == kindOperator && spacedOperators[tok.text] && endsOperand(prev):
				space = true
			case prev.kind == kindOperator && spacedOperators[prev.text] && startsOperand(tok) &&
				i >= 2 && endsOperand(tokens[i-2]):
				space = true
			default:
				space = tok.gap
			}
			// Nothing may follow a line comment on its line.
			if prev.kind == kindLineComment && newlineIndent < 0 {
				newlineIndent = lineIndent
			}
		}

		switch {
		case newlineIndent >= 0:
			out.WriteString("\n")
			if blank {
				out.WriteString("\n")
			}
			out.WriteString(strings.Repeat(indentUnit, newlineIndent))
			lineIndent = newlineIndent
		case space:
			out.WriteString(" ")
		}
		out.WriteString(tok.text)

		// Track what this token means for the tokens after it.
		if !isComment(tok) && !top.started {
			top.started = true
			top.first = tok.upper()
			top.clause = top.first
			if top.first == "SELECT" {
				markSelectList(tokens, i, breakAfter)
			}
		}
		switch tok.upper() {
		case "BETWEEN":
			top.between++
		case "CASE":
			top.caseDepth++
		case "END":
			if top.caseDepth > 0 {
				top.caseDepth--
			}
		}
		if tok.is(kindPunct, "(") {
			frames = append(frames, frame{
				subquery:    opensSubquery(tokens, i),
				base:        lineIndent + 1,
				closeIndent: lineIndent,
			})
		}
	}
	return out.String()
}

// opensSubquery reports whether the parenthesis at i wraps a statement.
func opensSubquery(tokens []token, i int) bool {
	for j := i + 1; j < len(tokens); j++ {
		if isComment(tokens[j]) {
			continue
		}
		return subqueryStarts[tokens[j].upper()]
	}
	return false
}

// clauseStart returns the name of the clause that starts at tokens[i], or ""
// when that word does not start a clause here.
func clauseStart(tokens []token, i int, top *frame) string {
	word := tokens[i].upper()
	previous := tokens[i-1].upper()
	next := ""
	if i+1 < len(tokens) {
		next = tokens[i+1].upper()
	}

	switch word {
	case "SELECT", "WHERE", "HAVING", "LIMIT", "RETURNING", "WINDOW", "UNION", "EXCEPT", "INTERSECT":
		return word
	case "FROM":
		// DELETE FROM and IS DISTINCT FROM are one phrase.
		if previous != "DELETE" && previous != "DISTINCT" {
			return word
		}
	case "VALUES":
		// MySQL also has a VALUES() function, used after the VALUES clause.
		if top.clause != "VALUES" && top.clause != "SET" {
			return word
		}
	case "GROUP", "ORDER":
		if next == "BY" {
			return word
		}
	case "SET":
		if (top.first == "UPDATE" || top.first == "MERGE") && previous != "UPDATE" {
			return word
		}
	case "JOIN", "APPLY":
		if !joinModifiers[previous] {
			return "JOIN"
		}
	case "INSERT", "UPDATE", "DELETE":
		// The statement after a list of common table expressions.
		if top.first == "WITH" && tokens[i-1].is(kindPunct, ")") {
			return word
		}
	default:
		if joinModifiers[word] && !joinModifiers[previous] {
			for j := i + 1; j < len(tokens); j++ {
				following := tokens[j].upper()
				if following == "JOIN" || following == "APPLY" {
					return "JOIN"
				}
				if !joinModifiers[following] {
					break
				}
			}
		}
	}
	return ""
}

// markSelectList decides whether the column list of the SELECT at tokens[i]
// is put one column per line, and records where the line breaks go.
func markSelectList(tokens []token, i int, breakAfter map[int]bool) {
	start := i + 1
	for start < len(tokens) && (tokens[start].upper() == "DISTINCT" || tokens[start].upper() == "ALL") {
		start++
	}

	var commas []int
	depth, width := 0, 0
scan:
	for j := start; j < len(tokens); j++ {
		tok := tokens[j]
		if tok.kind == kindPunct {
			switch tok.text {
			case "(":
				depth++
			case ")":
				if depth == 0 {
					break scan
				}
				depth--
			case ";":
				if depth == 0 {
					break scan
				}
			case ",":
				if depth == 0 {
					commas = append(commas, j)
				}
			}
		}
		if depth == 0 && selectListEnds[tok.upper()] && tokens[j-1].upper() != "DISTINCT" {
			break
		}
		// Independent of the current spacing, so formatting twice gives the
		// same layout.
		width += len(tok.text) + 1
	}

	if len(commas) == 0 || (len(commas) < selectListBreakItems-1 && width <= selectListBreakWidth) {
		return
	}
	breakAfter[start-1] = true
	for _, comma := range commas {
		breakAfter[comma] = true
	}
}
