package sqlformat

import (
	"strings"
	"testing"
)

var (
	postgres = Dialect{NestedComments: true, DollarQuotes: true}
	mysql    = Dialect{BackslashEscapes: true, HashComments: true}
	mssql    = Dialect{NestedComments: true, BracketIdentifiers: true}
)

func TestFormatLayout(t *testing.T) {
	tests := []struct {
		name    string
		dialect Dialect
		in      string
		want    string
	}{
		{
			name: "clauses joins and conditions",
			in:   "select u.id, u.name from users u left join orders o on o.user_id=u.id and o.paid where u.active = true and u.age between 18 and 30 or u.admin group by u.id order by u.name limit 10;",
			want: `select u.id, u.name
from users u
left join orders o on o.user_id = u.id
  and o.paid
where u.active = true
  and u.age between 18 and 30
  or u.admin
group by u.id
order by u.name
limit 10;`,
		},
		{
			name: "long select list goes one column per line",
			in:   "SELECT DISTINCT a, b, count(x, y) AS c, d FROM t",
			want: `SELECT DISTINCT
  a,
  b,
  count(x, y) AS c,
  d
FROM t`,
		},
		{
			name: "subquery and cte",
			in:   "with recent as (select * from orders where created > now()) select * from users where id in (select user_id from recent) and name in ('a','b')",
			want: `with recent as (
  select *
  from orders
  where created > now()
)
select *
from users
where id in (
  select user_id
  from recent
)
  and name in ('a', 'b')`,
		},
		{
			name: "clause words inside function calls stay inline",
			in:   "select extract(year from d), sum(a) over (partition by b order by c), count(*) filter (where x) from t",
			want: `select
  extract(year from d),
  sum(a) over (partition by b order by c),
  count(*) filter (where x)
from t`,
		},
		{
			name: "insert update delete and statement separation",
			in:   "insert into t (a,b) values (1,'x'),(2,'y'); update t set a=1, b=2 where c=3; delete from t where a is distinct from b",
			want: `insert into t (a, b)
values (1, 'x'), (2, 'y');

update t
set a = 1, b = 2
where c = 3;

delete from t
where a is distinct from b`,
		},
		{
			name: "case expressions keep their conditions inline",
			in:   "select a from t where case when a = 1 and b = 2 then true else false end and c",
			want: `select a
from t
where case when a = 1 and b = 2 then true else false end
  and c`,
		},
		{
			name: "comments",
			in:   "-- header\nselect a, -- first\n b from t /* why */ where x = 1 -- tail",
			want: `-- header
select a, -- first
b
from t /* why */
where x = 1 -- tail`,
		},
		{
			name:    "union",
			dialect: postgres,
			in:      "select 1 union all select 2",
			want:    "select 1\nunion all\nselect 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(tt.in, tt.dialect)
			if got != tt.want {
				t.Errorf("Format() =\n%s\n\nwant\n%s", got, tt.want)
			}
			if again := Format(got, tt.dialect); again != got {
				t.Errorf("formatting twice changed the result:\n%s", again)
			}
		})
	}
}

// Formatting must never change what a query means: tokens written without a
// space between them stay glued, and string contents are never touched.
func TestFormatKeepsTokensIntact(t *testing.T) {
	tests := []struct {
		name    string
		dialect Dialect
		in      string
		keep    []string
	}{
		{"parameters", postgres, "select a from t where b=:name and c=@p1 and d=$1 and e=? and f=%s and g=@@version and h=%(key)s",
			[]string{":name", "@p1", "$1", "=?", "%s", "@@version", "%(key)s"}},
		{"operators", postgres, "select a->>'k', b#>'{x}', f||g, i::int, -1, a*-1, j->'k'->>'z', arr[1], x@>y from t where a<=1 and b!=2",
			[]string{"a->>'k'", "b#>'{x}'", "i::int", "-1", "a*-1", "j->'k'->>'z'", "arr[1]", "x@>y", "a <= 1", "b != 2"}},
		{"string prefixes", postgres, `select E'a\'b  from t', N'uni  code', x'ff', _utf8'x' from t`,
			[]string{`E'a\'b  from t'`, "N'uni  code'", "x'ff'", "_utf8'x'"}},
		{"dollar quotes", postgres, "select $$body;  where x$$, $fn$ select  1 $fn$ from t",
			[]string{"$$body;  where x$$", "$fn$ select  1 $fn$"}},
		{"numbers", postgres, "select 1.5e10, .5, 0x1F, 1e-3 from t", []string{"1.5e10", ".5", "0x1F", "1e-3"}},
		{"standard strings ignore backslashes", postgres, `select 'a\', 'from  x' from t`, []string{`'a\'`, `'from  x'`}},
		{"mysql backslash escapes", mysql, `select 'it\'s  where', "q\"  from" from t # where  note`,
			[]string{`'it\'s  where'`, `"q\"  from"`, "# where  note"}},
		{"mysql double dash without space is arithmetic", mysql, "select 5--3 from t", []string{"5--3"}},
		{"quoted identifiers", mssql, "select [from  table].[where], \"a  b\", `c  d` from [dbo].[t]",
			[]string{"[from  table].[where]", `"a  b"`, "`c  d`", "[dbo].[t]"}},
		{"nested comments", postgres, "select 1 /* a /* from  b */ where */ from t", []string{"/* a /* from  b */ where */"}},
		{"doubled quotes", postgres, "select 'it''s  from' from t", []string{"'it''s  from'"}},
		{"unterminated string", postgres, "select 'never  ends from t", []string{"'never  ends from t"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tokenize(tt.in, tt.dialect)
			rendered := render(tokens)
			if !sameTokens(tokens, tokenize(rendered, tt.dialect)) {
				t.Fatalf("layout changed the tokens:\n%s", rendered)
			}
			got := Format(tt.in, tt.dialect)
			for _, keep := range tt.keep {
				if !strings.Contains(got, keep) {
					t.Errorf("Format() lost %q:\n%s", keep, got)
				}
			}
		})
	}
}

func TestFormatEdgeCases(t *testing.T) {
	for _, in := range []string{"", "   \n ", ")", "((", "select", ";;", "select (", "a ) b ) c"} {
		got := Format(in, Dialect{})
		if !sameTokens(tokenize(in, Dialect{}), tokenize(got, Dialect{})) {
			t.Errorf("Format(%q) = %q changed the tokens", in, got)
		}
	}
	if got := Format("  \n", Dialect{}); got != "  \n" {
		t.Errorf("blank input = %q, want it unchanged", got)
	}
}
