package filemap_test

import (
	"strings"
	"testing"

	filemap "github.com/kaushal/skim/internal/filemap"
)

// TestGenerate_Elixir_BasicModule verifies a simple module with def/defp functions.
func TestGenerate_Elixir_BasicModule(t *testing.T) {
	src := `defmodule MyApp.Greeter do
  @moduledoc "Greeting utilities."

  def hello(name) do
    "Hello, #{name}!"
  end

  def goodbye(name), do: "Goodbye, #{name}!"

  defp private_helper(x), do: x * 2
end
`
	fm, ok := filemap.Generate(src, "greeter.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.HasPrefix(kind, "defmodule MyApp.Greeter") {
		t.Errorf("unexpected Kind prefix: %s", kind)
	}
	for _, fn := range []string{"hello", "goodbye", "private_helper"} {
		if !strings.Contains(kind, fn) {
			t.Errorf("expected %q in Kind, got: %s", fn, kind)
		}
	}
	if !strings.Contains(fm.Summary, "1 module") {
		t.Errorf("unexpected Summary: %s", fm.Summary)
	}
}

// TestGenerate_Elixir_MultipleModules verifies two modules in one file.
func TestGenerate_Elixir_MultipleModules(t *testing.T) {
	src := `defmodule MyApp.Repo do
  use Ecto.Repo, otp_app: :my_app

  def get!(schema, id), do: Ecto.Repo.get!(schema, id)
  def insert!(changeset), do: Ecto.Repo.insert!(changeset)
end

defmodule MyApp.Repo.Migrations.CreateUsers do
  use Ecto.Migration

  def change do
    create table(:users) do
      add :name, :string
      add :email, :string
      timestamps()
    end
  end
end
`
	fm, ok := filemap.Generate(src, "repo.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "MyApp.Repo") {
		t.Errorf("unexpected first entry: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "MyApp.Repo.Migrations.CreateUsers") {
		t.Errorf("unexpected second entry: %s", fm.Map[1].Kind)
	}
	if !strings.Contains(fm.Summary, "2 modules") {
		t.Errorf("unexpected Summary: %s", fm.Summary)
	}
}

// TestGenerate_Elixir_PatternMatchClauses verifies that multi-clause functions
// are deduplicated (pattern matching is idiomatic Elixir, so "def foo" may
// appear many times — the map shows the name once).
func TestGenerate_Elixir_PatternMatchClauses(t *testing.T) {
	src := `defmodule MyApp.Parser do
  def parse({:ok, result}), do: {:parsed, result}
  def parse({:error, reason}), do: {:failed, reason}
  def parse(_other), do: :unknown

  def validate(value) when is_integer(value), do: :ok
  def validate(value) when is_binary(value), do: :ok
  def validate(_), do: {:error, :invalid}
end
`
	fm, ok := filemap.Generate(src, "parser.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	kind := fm.Map[0].Kind
	// Both names should appear exactly once
	if strings.Count(kind, "parse") != 1 {
		t.Errorf("expected 'parse' once, got: %s", kind)
	}
	if strings.Count(kind, "validate") != 1 {
		t.Errorf("expected 'validate' once, got: %s", kind)
	}
	// No overflow: only 2 unique names
	if strings.Contains(kind, "(+") {
		t.Errorf("unexpected overflow in Kind: %s", kind)
	}
}

// TestGenerate_Elixir_Protocol verifies defprotocol is recognized.
func TestGenerate_Elixir_Protocol(t *testing.T) {
	src := `defprotocol MyApp.Serializable do
  @doc "Serialize a value to JSON."
  def serialize(value)

  @doc "Deserialize from a map."
  def deserialize(map)
end
`
	fm, ok := filemap.Generate(src, "serializable.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	kind := fm.Map[0].Kind
	if !strings.HasPrefix(kind, "defprotocol MyApp.Serializable") {
		t.Errorf("unexpected Kind: %s", kind)
	}
	for _, fn := range []string{"serialize", "deserialize"} {
		if !strings.Contains(kind, fn) {
			t.Errorf("expected %q in Kind, got: %s", fn, kind)
		}
	}
	if !strings.Contains(fm.Summary, "1 protocol") {
		t.Errorf("unexpected Summary: %s", fm.Summary)
	}
}

// TestGenerate_Elixir_Impl verifies defimpl is recognized.
func TestGenerate_Elixir_Impl(t *testing.T) {
	src := `defimpl Jason.Encoder, for: MyApp.User do
  def encode(user, opts) do
    Jason.Encode.map(%{name: user.name, email: user.email}, opts)
  end
end
`
	fm, ok := filemap.Generate(src, "user_encoder.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	kind := fm.Map[0].Kind
	if !strings.HasPrefix(kind, "defimpl Jason.Encoder for MyApp.User") {
		t.Errorf("unexpected Kind: %s", kind)
	}
	if !strings.Contains(kind, "encode") {
		t.Errorf("expected 'encode' in Kind, got: %s", kind)
	}
	if !strings.Contains(fm.Summary, "1 impl") {
		t.Errorf("unexpected Summary: %s", fm.Summary)
	}
}

// TestGenerate_Elixir_ImplForTarget verifies that two defimpl blocks for the
// same protocol but different target types produce distinguishable Kind labels.
// Previously the label only included the protocol name, making them identical.
func TestGenerate_Elixir_ImplForTarget(t *testing.T) {
	src := `defimpl Jason.Encoder, for: MyApp.User do
  def encode(user, opts), do: Jason.Encode.map(%{name: user.name}, opts)
end

defimpl Jason.Encoder, for: MyApp.Order do
  def encode(order, opts), do: Jason.Encode.map(%{id: order.id}, opts)
end
`
	fm, ok := filemap.Generate(src, "encoders.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "Jason.Encoder for MyApp.User") {
		t.Errorf("expected 'for MyApp.User' in first entry, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "Jason.Encoder for MyApp.Order") {
		t.Errorf("expected 'for MyApp.Order' in second entry, got: %s", fm.Map[1].Kind)
	}
	// Labels must be distinguishable.
	if fm.Map[0].Kind == fm.Map[1].Kind {
		t.Errorf("two defimpl entries have identical Kind labels: %s", fm.Map[0].Kind)
	}
}

// TestGenerate_Elixir_MethodOverflow verifies (+N) overflow for more than 4 defs.
func TestGenerate_Elixir_MethodOverflow(t *testing.T) {
	src := `defmodule MyApp.Router do
  def get(path, conn), do: {:get, path, conn}
  def post(path, conn), do: {:post, path, conn}
  def put(path, conn), do: {:put, path, conn}
  def patch(path, conn), do: {:patch, path, conn}
  def delete(path, conn), do: {:delete, path, conn}
  def options(path, conn), do: {:options, path, conn}
end
`
	fm, ok := filemap.Generate(src, "router.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+2)") {
		t.Errorf("expected (+2) overflow for 6 defs, got: %s", kind)
	}
}

// TestGenerate_Elixir_StringsWithDo verifies that "do" inside string literals
// does not increment block depth, so brace tracking stays correct.
func TestGenerate_Elixir_StringsWithDo(t *testing.T) {
	src := `defmodule MyApp.Templates do
  def render(:welcome, name) do
    msg = "Welcome! Please do continue."
    footer = "end of message"
    {:ok, "#{msg} #{footer}, #{name}"}
  end

  def render(:error, reason), do: {:error, reason}
end
`
	fm, ok := filemap.Generate(src, "templates.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "render") {
		t.Errorf("expected 'render' in Kind, got: %s", kind)
	}
}

// TestGenerate_Elixir_DoKeywordForm verifies that "do:" keyword pairs (not
// block openers) don't disturb depth, so one-liner def/defp are collected.
func TestGenerate_Elixir_DoKeywordForm(t *testing.T) {
	src := `defmodule MyApp.Math do
  def add(a, b), do: a + b
  def subtract(a, b), do: a - b
  defp multiply(a, b), do: a * b
  def divide(_a, 0), do: {:error, :division_by_zero}
  def divide(a, b), do: {:ok, a / b}
end
`
	fm, ok := filemap.Generate(src, "math.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	kind := fm.Map[0].Kind
	for _, fn := range []string{"add", "subtract", "multiply", "divide"} {
		if !strings.Contains(kind, fn) {
			t.Errorf("expected %q in Kind, got: %s", fn, kind)
		}
	}
}

// TestGenerate_Elixir_PredBangSuffix verifies that ? and ! in function names
// are captured correctly (Elixir naming convention for predicates and bangs).
func TestGenerate_Elixir_PredBangSuffix(t *testing.T) {
	src := `defmodule MyApp.Validator do
  def valid?(value), do: is_binary(value) and byte_size(value) > 0
  def validate!(value) do
    if valid?(value), do: value, else: raise ArgumentError
  end
  defp check_length?(value, min), do: byte_size(value) >= min
end
`
	fm, ok := filemap.Generate(src, "validator.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	kind := fm.Map[0].Kind
	for _, fn := range []string{"valid?", "validate!", "check_length?"} {
		if !strings.Contains(kind, fn) {
			t.Errorf("expected %q in Kind, got: %s", fn, kind)
		}
	}
}

// TestGenerate_Elixir_MultilineHeader verifies that a defmodule whose "do"
// appears on a separate line is still parsed correctly.
func TestGenerate_Elixir_MultilineHeader(t *testing.T) {
	src := `defmodule MyApp.LongModule
  do
  def init(state), do: {:ok, state}
  def handle_call(req, _from, state), do: {:reply, :ok, state}
end
`
	fm, ok := filemap.Generate(src, "long.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "init") || !strings.Contains(kind, "handle_call") {
		t.Errorf("expected 'init' and 'handle_call' in Kind, got: %s", kind)
	}
}

// TestGenerate_Elixir_LineRanges verifies that Map entries have correct line
// ranges covering the full module from defmodule to end.
func TestGenerate_Elixir_LineRanges(t *testing.T) {
	src := `defmodule A do
  def one, do: 1
end

defmodule B do
  def two, do: 2
  def three, do: 3
end
`
	fm, ok := filemap.Generate(src, "ab.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(fm.Map))
	}
	if fm.Map[0].Lines != "1-3" {
		t.Errorf("A: expected lines 1-3, got %s", fm.Map[0].Lines)
	}
	if fm.Map[1].Lines != "5-8" {
		t.Errorf("B: expected lines 5-8, got %s", fm.Map[1].Lines)
	}
}

// TestGenerate_Elixir_GeneratedFile verifies that generated files still produce
// a map (so the caller gets structural context) but the summary flags them.
func TestGenerate_Elixir_GeneratedFile(t *testing.T) {
	src := `# This file was generated by mix.
defmodule MyApp.Generated do
  def foo, do: :bar
end
`
	fm, ok := filemap.Generate(src, "generated.ex")
	if !ok {
		t.Fatal("expected deterministic map for generated Elixir file")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in summary, got: %s", fm.Summary)
	}
}

// TestGenerate_Elixir_CharlistDoEnd verifies that 'end' and 'do' inside
// single-quoted charlists do not corrupt block depth. Previously elixirNetBlocks
// skipped double-quoted strings but not charlists, so 'end' inside a charlist
// was counted as a block terminator, closing the enclosing module prematurely.
func TestGenerate_Elixir_CharlistDoEnd(t *testing.T) {
	src := `defmodule Example do
  def message do
    IO.puts('end')
    IO.puts('do')
    :ok
  end
end

defmodule Another do
  def run, do: :ok
end
`
	fm, ok := filemap.Generate(src, "charlist.ex")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 modules, got %d: %v", len(fm.Map), fm.Map)
	}
	// Both modules must have correct line ranges.
	if fm.Map[0].Lines != "1-7" {
		t.Errorf("Example: expected lines 1-7, got %s", fm.Map[0].Lines)
	}
	if fm.Map[1].Lines != "9-11" {
		t.Errorf("Another: expected lines 9-11, got %s", fm.Map[1].Lines)
	}
	if !strings.Contains(fm.Map[0].Kind, "message") {
		t.Errorf("expected 'message' in Example Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "run") {
		t.Errorf("expected 'run' in Another Kind, got: %s", fm.Map[1].Kind)
	}
}

// TestGenerate_Elixir_FallsThrough_Empty verifies that empty files fall through.
func TestGenerate_Elixir_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "empty.ex")
	if ok {
		t.Error("expected empty file to fall through")
	}
}

// TestGenerate_Elixir_FallsThrough_NoStructure verifies that script files with
// no defmodule/defprotocol/defimpl fall through.
func TestGenerate_Elixir_FallsThrough_NoStructure(t *testing.T) {
	src := `# A mix task script
IO.puts("hello")
x = 1 + 2
IO.inspect(x)
`
	_, ok := filemap.Generate(src, "script.exs")
	if ok {
		t.Error("expected no-structure file to fall through")
	}
}

// TestGenerate_Elixir_ExsExtension verifies that .exs files (scripts) work the
// same as .ex files when they do contain module definitions.
func TestGenerate_Elixir_ExsExtension(t *testing.T) {
	src := `defmodule MyApp.MixProject do
  use Mix.Project

  def project do
    [
      app: :my_app,
      version: "0.1.0"
    ]
  end

  def deps do
    [{:jason, "~> 1.4"}]
  end
end
`
	fm, ok := filemap.Generate(src, "mix.exs")
	if !ok {
		t.Fatal("expected deterministic map for .exs file")
	}
	kind := fm.Map[0].Kind
	if !strings.HasPrefix(kind, "defmodule MyApp.MixProject") {
		t.Errorf("unexpected Kind: %s", kind)
	}
	for _, fn := range []string{"project", "deps"} {
		if !strings.Contains(kind, fn) {
			t.Errorf("expected %q in Kind, got: %s", fn, kind)
		}
	}
}
