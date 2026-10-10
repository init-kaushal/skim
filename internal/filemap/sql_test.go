package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_SQL_CreateTable(t *testing.T) {
	src := `CREATE TABLE users (
    id          BIGSERIAL    PRIMARY KEY,
    email       TEXT         NOT NULL UNIQUE,
    name        TEXT         NOT NULL,
    created_at  TIMESTAMPTZ  DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE TABLE sessions (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ  DEFAULT NOW(),
    expires_at TIMESTAMPTZ  NOT NULL
);
`
	fm, ok := filemap.Generate(src, "schema.sql")
	if !ok {
		t.Fatal("expected deterministic map for SQL file")
	}

	if !strings.Contains(fm.Summary, "table") {
		t.Errorf("expected 'table' in summary, got: %s", fm.Summary)
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(fm.Map))
	}

	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	if !symbolSet["table users"] {
		t.Errorf("expected 'table users' in %v", fm.Symbols)
	}
	if !symbolSet["table sessions"] {
		t.Errorf("expected 'table sessions' in %v", fm.Symbols)
	}
}

func TestGenerate_SQL_MigrationAlterTable(t *testing.T) {
	src := `ALTER TABLE users ADD COLUMN last_login TIMESTAMPTZ;

ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);

ALTER TABLE sessions DROP COLUMN IF EXISTS legacy_token;
`
	fm, ok := filemap.Generate(src, "0012_add_last_login.sql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if !strings.Contains(fm.Summary, "SQL migration") {
		t.Errorf("expected 'SQL migration' prefix, got: %s", fm.Summary)
	}
	if len(fm.Map) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	// Kind labels use "alter table" prefix
	for _, e := range fm.Map {
		if !strings.HasPrefix(e.Kind, "alter table") {
			t.Errorf("expected 'alter table' prefix, got: %s", e.Kind)
		}
	}
}

func TestGenerate_SQL_Indexes(t *testing.T) {
	src := `CREATE INDEX idx_users_email ON users (email);

CREATE UNIQUE INDEX idx_users_email_lower ON users (lower(email));

CREATE INDEX CONCURRENTLY idx_sessions_user_id ON sessions (user_id);
`
	fm, ok := filemap.Generate(src, "indexes.sql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 3 {
		t.Fatalf("expected 3 index entries, got %d: %v", len(fm.Map), fm.Map)
	}
	// Each entry should show the table name via "on <table>"
	for _, e := range fm.Map {
		if !strings.Contains(e.Kind, " on ") {
			t.Errorf("expected 'on <table>' in index label, got: %s", e.Kind)
		}
	}
}

func TestGenerate_SQL_Views(t *testing.T) {
	src := `CREATE OR REPLACE VIEW active_users AS
    SELECT id, email, name
    FROM users
    WHERE deleted_at IS NULL;

CREATE VIEW user_session_counts AS
    SELECT u.id, u.email, COUNT(s.id) AS session_count
    FROM users u
    LEFT JOIN sessions s ON s.user_id = u.id
    GROUP BY u.id, u.email;
`
	fm, ok := filemap.Generate(src, "views.sql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 view entries, got %d: %v", len(fm.Map), fm.Map)
	}
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	if !symbolSet["view active_users"] {
		t.Errorf("expected 'view active_users' in %v", fm.Symbols)
	}
}

func TestGenerate_SQL_PostgresFunction(t *testing.T) {
	// Dollar-quoted function bodies contain semicolons; they must not close
	// the statement early.
	src := `CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION audit_log()
RETURNS TRIGGER AS $$
DECLARE
    v_old JSONB;
    v_new JSONB;
BEGIN
    v_old := to_jsonb(OLD);
    v_new := to_jsonb(NEW);
    INSERT INTO audit_log (table_name, old_data, new_data) VALUES (TG_TABLE_NAME, v_old, v_new);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
`
	fm, ok := filemap.Generate(src, "functions.sql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 function entries (dollar-quote tracking broken?), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Symbols[0], "update_updated_at") {
		t.Errorf("expected function name in symbol, got: %v", fm.Symbols)
	}
}

// TestGenerate_SQL_PostgresFunctionInlineAS is the regression test for the bug
// where the opening $$ delimiter on the same line as CREATE FUNCTION was not
// detected, causing the semicolon inside the body to close the statement early.
func TestGenerate_SQL_PostgresFunctionInlineAS(t *testing.T) {
	src := `CREATE FUNCTION update_user() RETURNS trigger AS $$
BEGIN
    UPDATE users SET updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION another_func() RETURNS void AS $$
BEGIN
    DELETE FROM sessions WHERE expires_at < NOW();
END;
$$ LANGUAGE plpgsql;
`
	fm, ok := filemap.Generate(src, "triggers.sql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 function entries (inline AS $$ bug: semicolon in body closed statement early), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Symbols[0], "update_user") {
		t.Errorf("expected update_user in first symbol, got: %v", fm.Symbols)
	}
}

func TestGenerate_SQL_LineRanges(t *testing.T) {
	src := `CREATE TABLE alpha (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE beta (
    id BIGSERIAL PRIMARY KEY,
    value INT
);
`
	fm, ok := filemap.Generate(src, "schema.sql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(fm.Map))
	}
	// alpha starts at line 1
	if !strings.HasPrefix(fm.Map[0].Lines, "1") {
		t.Errorf("alpha should start at line 1, got: %s", fm.Map[0].Lines)
	}
	// beta starts at line 6
	if !strings.HasPrefix(fm.Map[1].Lines, "6") {
		t.Errorf("beta should start at line 6, got: %s", fm.Map[1].Lines)
	}
}

func TestGenerate_SQL_GeneratedFile(t *testing.T) {
	src := `-- pg_dump output generated by pg_dump version 15.3
-- DO NOT EDIT

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY
);
`
	fm, ok := filemap.Generate(src, "dump.sql")
	if !ok {
		t.Fatal("expected deterministic map for generated SQL")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_SQL_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "schema.sql")
	if ok {
		t.Error("expected fallthrough for empty SQL file")
	}
}

func TestGenerate_SQL_FallsThrough_NoDDL(t *testing.T) {
	// A file with only comments and SET statements has no DDL.
	src := `-- database configuration
SET statement_timeout = 0;
SET lock_timeout = 0;
SET client_encoding = 'UTF8';
`
	_, ok := filemap.Generate(src, "setup.sql")
	if ok {
		t.Error("expected fallthrough for SQL file with no DDL statements")
	}
}

func TestGenerate_SQL_QuotedIdentifiers(t *testing.T) {
	// Double-quoted identifiers (used when names have special chars or are reserved words)
	src := `CREATE TABLE "order" (
    id   BIGSERIAL PRIMARY KEY,
    name TEXT
);

CREATE INDEX "idx_order_name" ON "order" (name);
`
	fm, ok := filemap.Generate(src, "reserved.sql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	// Quoted identifiers should be unquoted in labels
	if !strings.Contains(fm.Map[0].Kind, "order") {
		t.Errorf("expected 'order' (unquoted) in table label, got: %s", fm.Map[0].Kind)
	}
}

func TestGenerate_SQL_MixedDDL(t *testing.T) {
	// A schema file with tables, indexes, and views produces a combined summary.
	src := `CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    price NUMERIC(10,2) NOT NULL
);

CREATE TABLE categories (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE INDEX idx_products_name ON products (name);

CREATE VIEW cheap_products AS
    SELECT * FROM products WHERE price < 10.00;
`
	fm, ok := filemap.Generate(src, "schema.sql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	// 4 statements: 2 tables + 1 index + 1 view
	if len(fm.Map) != 4 {
		t.Fatalf("expected 4 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	// Summary should contain all types
	for _, want := range []string{"table", "index", "view"} {
		if !strings.Contains(fm.Summary, want) {
			t.Errorf("expected %q in summary %q", want, fm.Summary)
		}
	}
}
