package filemap

import (
	"strings"
	"testing"
)

// TestGenerate_Erlang_Basic: typical OTP gen_server module.
func TestGenerate_Erlang_Basic(t *testing.T) {
	src := `-module(my_server).
-behaviour(gen_server).

-export([start_link/0, stop/1]).
-export([init/1, handle_call/3, handle_cast/2, terminate/2]).

-record(state, {count = 0 :: non_neg_integer()}).

start_link() ->
    gen_server:start_link(?MODULE, [], []).

stop(Pid) ->
    gen_server:stop(Pid).

init([]) ->
    {ok, #state{}}.

handle_call(get_count, _From, State) ->
    {reply, State#state.count, State};
handle_call(_Req, _From, State) ->
    {reply, ok, State}.

handle_cast(increment, State) ->
    {noreply, State#state{count = State#state.count + 1}}.

terminate(_Reason, _State) ->
    ok.
`
	fm, ok := Generate(src, "my_server.erl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "Erlang") {
		t.Errorf("summary %q missing Erlang", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "my_server") {
		t.Errorf("summary %q missing module name", fm.Summary)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "state") {
		t.Errorf("kind %q missing record state", kind)
	}
	if !strings.Contains(kind, "start_link") {
		t.Errorf("kind %q missing start_link", kind)
	}
	// Multiple clauses of the same function should appear only once (across
	// all symbols, not just the truncated kind label).
	count := 0
	for _, s := range fm.Symbols {
		if s == "fn handle_call" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("fn handle_call should appear once in symbols, got %d in %v", count, fm.Symbols)
	}
	// Symbols must include record prefix.
	hasRecordState := false
	for _, s := range fm.Symbols {
		if s == "record state" {
			hasRecordState = true
		}
	}
	if !hasRecordState {
		t.Errorf("symbols %v missing 'record state'", fm.Symbols)
	}
}

// TestGenerate_Erlang_Types: -type and -opaque declarations.
func TestGenerate_Erlang_Types(t *testing.T) {
	src := `-module(shapes).

-type point() :: {float(), float()}.
-opaque circle() :: {point(), float()}.

area(Circle) ->
    {_, R} = Circle,
    math:pi() * R * R.
`
	fm, ok := Generate(src, "shapes.erl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	hasTypePoint := false
	hasTypeCircle := false
	for _, s := range fm.Symbols {
		if s == "type point" {
			hasTypePoint = true
		}
		if s == "type circle" {
			hasTypeCircle = true
		}
	}
	if !hasTypePoint {
		t.Errorf("symbols %v missing 'type point'", fm.Symbols)
	}
	if !hasTypeCircle {
		t.Errorf("symbols %v missing 'type circle' (from -opaque)", fm.Symbols)
	}
}

// TestGenerate_Erlang_CommentSkip: lines starting with % are ignored.
func TestGenerate_Erlang_CommentSkip(t *testing.T) {
	src := `-module(commented).

%% foo(X) -> X.
%% -record(fake, {}).

real_fn(X) -> X.
`
	fm, ok := Generate(src, "commented.erl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	for _, s := range fm.Symbols {
		if strings.Contains(s, "foo") {
			t.Errorf("symbols should not contain foo (commented out), got %v", fm.Symbols)
		}
		if strings.Contains(s, "fake") {
			t.Errorf("symbols should not contain fake record (commented out), got %v", fm.Symbols)
		}
	}
	found := false
	for _, s := range fm.Symbols {
		if s == "fn real_fn" {
			found = true
		}
	}
	if !found {
		t.Errorf("symbols %v missing 'fn real_fn'", fm.Symbols)
	}
}

// TestGenerate_Erlang_IndentedSkip: function body lines at non-zero indent
// are not mistaken for function declarations.
func TestGenerate_Erlang_IndentedSkip(t *testing.T) {
	src := `-module(guard_test).

classify(N) when N > 0 ->
    positive;
classify(0) ->
    zero;
classify(_) ->
    negative.

identity(X) -> X.
`
	fm, ok := Generate(src, "guard_test.erl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	// Only 'classify' and 'identity' should appear as fns — not 'positive',
	// 'zero', 'negative' (which are atoms in the function body).
	for _, s := range fm.Symbols {
		if s == "fn positive" || s == "fn zero" || s == "fn negative" {
			t.Errorf("indented atom %q incorrectly collected as function", s)
		}
	}
	classifyFound := false
	for _, s := range fm.Symbols {
		if s == "fn classify" {
			classifyFound = true
		}
	}
	if !classifyFound {
		t.Errorf("symbols %v missing 'fn classify'", fm.Symbols)
	}
}

// TestGenerate_Erlang_Header: .hrl file collects records and macros.
func TestGenerate_Erlang_Header(t *testing.T) {
	src := `%% Common definitions.

-record(user, {id, name, email}).
-record(session, {token, user_id, created_at}).

-define(DEFAULT_TIMEOUT, 5000).
-define(MAX_RETRIES, 3).
-define(APP_NAME, my_app).
`
	fm, ok := Generate(src, "common.hrl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "Erlang header") {
		t.Errorf("summary %q missing 'Erlang header'", fm.Summary)
	}
	hasUser := false
	hasTimeout := false
	for _, s := range fm.Symbols {
		if s == "record user" {
			hasUser = true
		}
		if s == "macro DEFAULT_TIMEOUT" {
			hasTimeout = true
		}
	}
	if !hasUser {
		t.Errorf("symbols %v missing 'record user'", fm.Symbols)
	}
	if !hasTimeout {
		t.Errorf("symbols %v missing 'macro DEFAULT_TIMEOUT'", fm.Symbols)
	}
}

// TestGenerate_Erlang_Header_NoFunctions: functions in .hrl are not collected.
func TestGenerate_Erlang_Header_NoFunctions(t *testing.T) {
	src := `-define(MACRO, value).
-record(point, {x, y}).
`
	fm, ok := Generate(src, "defs.hrl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	for _, s := range fm.Symbols {
		if strings.HasPrefix(s, "fn ") {
			t.Errorf("functions should not appear in .hrl symbols, got %q", s)
		}
	}
}

// TestGenerate_Erlang_OverflowLabel: more than 4 items uses +N notation.
func TestGenerate_Erlang_OverflowLabel(t *testing.T) {
	src := `-module(many).

fn1() -> ok.
fn2() -> ok.
fn3() -> ok.
fn4() -> ok.
fn5() -> ok.
fn6() -> ok.
`
	fm, ok := Generate(src, "many.erl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Map[0].Kind, "(+") {
		t.Errorf("kind %q should have overflow notation", fm.Map[0].Kind)
	}
}

// TestGenerate_Erlang_Generated: generated file note in summary.
func TestGenerate_Erlang_Generated(t *testing.T) {
	src := `%% DO NOT EDIT — generated by protoc-gen-erl
-module(proto_generated).
stub() -> ok.
`
	fm, ok := Generate(src, "proto_generated.erl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("summary %q missing generated note", fm.Summary)
	}
}

// TestGenerate_Erlang_ModuleNameLabel: module name from -module() used as label base.
func TestGenerate_Erlang_ModuleNameLabel(t *testing.T) {
	src := `-module(my_module).
hello() -> world.
`
	fm, ok := Generate(src, "some_file.erl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Map[0].Kind, "my_module") {
		t.Errorf("kind %q should use module name, not file stem", fm.Map[0].Kind)
	}
}

// TestGenerate_Erlang_MultiClauseDedup: same function name from multiple
// clauses appears only once in symbols.
func TestGenerate_Erlang_MultiClauseDedup(t *testing.T) {
	src := `-module(dedup).

fib(0) -> 0;
fib(1) -> 1;
fib(N) -> fib(N-1) + fib(N-2).
`
	fm, ok := Generate(src, "dedup.erl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	count := 0
	for _, s := range fm.Symbols {
		if s == "fn fib" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("fn fib should appear once, got %d in symbols %v", count, fm.Symbols)
	}
}

// TestGenerate_Erlang_FallsThrough_Empty: empty file falls through.
func TestGenerate_Erlang_FallsThrough_Empty(t *testing.T) {
	_, ok := Generate("", "empty.erl")
	if ok {
		t.Error("expected ok=false for empty file")
	}
}

// TestGenerate_Erlang_FallsThrough_NoStructure: plain text falls through.
func TestGenerate_Erlang_FallsThrough_NoStructure(t *testing.T) {
	_, ok := Generate("just some text", "plain.erl")
	if ok {
		t.Error("expected ok=false for plain text")
	}
}

// TestGenerate_Erlang_LineRange: single map entry covers the whole file.
func TestGenerate_Erlang_LineRange(t *testing.T) {
	src := "-module(m).\nhello() -> world.\n"
	fm, ok := Generate(src, "m.erl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 map entry, got %d", len(fm.Map))
	}
	if fm.Map[0].Lines != "1-2" {
		t.Errorf("Lines = %q, want 1-2", fm.Map[0].Lines)
	}
}
