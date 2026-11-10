package drivers

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestTableDDLSQLiteIncludesIndexes(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, statement := range []string{
		"CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)",
		"CREATE INDEX users_name ON users (name)",
		"CREATE TABLE other (id INTEGER)",
	} {
		if _, err := conn.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	sqlite := &SQLite{Connection: conn, Provider: DriverSqlite}

	ddl, err := TableDDL(context.Background(), sqlite, "", "users")
	if err != nil {
		t.Fatal(err)
	}
	want := "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);\n\nCREATE INDEX users_name ON users (name);"
	if ddl != want {
		t.Errorf("ddl = %q, want %q", ddl, want)
	}

	if _, err := TableDDL(context.Background(), sqlite, "", "missing"); err == nil {
		t.Error("missing table: error = nil")
	}
	if _, err := TableDDL(context.Background(), sqlite, "", "x' OR '1'='1"); err == nil {
		t.Error("a quote in the table name matched rows")
	}
}

func TestTableDDLMySQL(t *testing.T) {
	db, mock := newExplainMock(t)
	mysql := &MySQL{Connection: db, Provider: DriverMySQL, CurrentDatabase: "shop"}

	mock.ExpectQuery("SHOW CREATE TABLE `shop`.`or``ders`").
		WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).AddRow("orders", "CREATE TABLE `orders` (\n  `id` int\n)"))

	ddl, err := TableDDL(context.Background(), mysql, "shop", "or`ders")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ddl, "CREATE TABLE `orders`") {
		t.Errorf("ddl = %q", ddl)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestTableDDLPostgresRebuildsTable(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	pg := &Postgres{Connection: db, Provider: DriverPostgres, CurrentDatabase: "app"}

	mock.ExpectQuery(`SELECT relkind FROM pg_class WHERE oid = '"public"\."us""ers"'::regclass`).
		WillReturnRows(sqlmock.NewRows([]string{"relkind"}).AddRow("r"))
	mock.ExpectQuery(`FROM pg_attribute`).
		WillReturnRows(sqlmock.NewRows([]string{"attname", "format_type", "attnotnull", "default", "attidentity"}).
			AddRow("id", "integer", "true", "", "a").
			AddRow("name", "character varying(50)", "false", "'x'::character varying", ""))
	mock.ExpectQuery(`FROM pg_constraint`).
		WillReturnRows(sqlmock.NewRows([]string{"conname", "def"}).AddRow("users_pkey", "PRIMARY KEY (id)"))
	mock.ExpectQuery(`FROM pg_indexes`).
		WillReturnRows(sqlmock.NewRows([]string{"indexdef"}).AddRow("CREATE INDEX users_name ON public.users USING btree (name)"))

	ddl, err := TableDDL(context.Background(), pg, "app", `public.us"ers`)
	if err != nil {
		t.Fatal(err)
	}
	want := reconstructedNote + `CREATE TABLE "public"."us""ers" (
  "id" integer GENERATED ALWAYS AS IDENTITY NOT NULL,
  "name" character varying(50) DEFAULT 'x'::character varying,
  CONSTRAINT "users_pkey" PRIMARY KEY (id)
);

CREATE INDEX users_name ON public.users USING btree (name);`
	if ddl != want {
		t.Errorf("ddl =\n%s\n\nwant\n%s", ddl, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestMSSQLColumnType(t *testing.T) {
	tests := []struct{ name, length, precision, scale, want string }{
		{"nvarchar", "100", "0", "0", "nvarchar(50)"},
		{"nvarchar", "-1", "0", "0", "nvarchar(MAX)"},
		{"varchar", "20", "0", "0", "varchar(20)"},
		{"decimal", "9", "18", "2", "decimal(18, 2)"},
		{"datetime2", "8", "27", "7", "datetime2(7)"},
		{"int", "4", "10", "0", "int"},
	}
	for _, tt := range tests {
		if got := mssqlColumnType(tt.name, tt.length, tt.precision, tt.scale); got != tt.want {
			t.Errorf("mssqlColumnType(%s, %s) = %q, want %q", tt.name, tt.length, got, tt.want)
		}
	}
}

func TestServerInfoSQLite(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	sqlite := &SQLite{Connection: conn, Provider: DriverSqlite}

	items, err := ServerInfo(context.Background(), sqlite)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].Value != DriverSqlite || items[1].Label != "Version" || items[1].Value == "" || items[2].Value != "UTF-8" {
		t.Errorf("items = %+v", items)
	}
}

func TestServerInfoKeepsFirstLineOfVersionBanner(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mssql := &MSSQL{Connection: db, Provider: DriverMSSQL}

	mock.ExpectQuery("@@VERSION").WillReturnRows(
		sqlmock.NewRows([]string{"v", "u", "d", "s"}).AddRow("Microsoft SQL Server 2022\n\tCopyright", "sa", "master", "box"))

	items, err := ServerInfo(context.Background(), mssql)
	if err != nil {
		t.Fatal(err)
	}
	if items[1].Value != "Microsoft SQL Server 2022" || items[4].Value != "box" {
		t.Errorf("items = %+v", items)
	}
}
