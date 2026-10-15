package filemap

import (
	"strings"
	"testing"
)

// TestGenerate_Lua_BasicFunctions: simple named function declarations.
func TestGenerate_Lua_BasicFunctions(t *testing.T) {
	src := `
function load()
  player = {x=100, y=100}
end

function update(dt)
  player.x = player.x + dt
end

function draw()
  love.graphics.rectangle("fill", player.x, player.y, 20, 20)
end
`
	fm, ok := Generate(src, "game.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "module game") {
		t.Errorf("kind %q missing 'module game'", kind)
	}
	for _, fn := range []string{"load", "update", "draw"} {
		if !strings.Contains(kind, fn) {
			t.Errorf("kind %q missing function %q", kind, fn)
		}
	}
}

// TestGenerate_Lua_LocalFunctions: local function declarations.
func TestGenerate_Lua_LocalFunctions(t *testing.T) {
	src := `
local function parse_args(s)
  return {}
end

local function run(args)
  return 0
end
`
	fm, ok := Generate(src, "cli.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "parse_args") {
		t.Errorf("kind %q missing 'parse_args'", kind)
	}
	if !strings.Contains(kind, "run") {
		t.Errorf("kind %q missing 'run'", kind)
	}
}

// TestGenerate_Lua_ModuleMethods: function M.name() style.
func TestGenerate_Lua_ModuleMethods(t *testing.T) {
	src := `
local M = {}

function M.new(opts)
  return setmetatable({opts=opts}, M)
end

function M.fetch(self, url)
  return nil
end

return M
`
	fm, ok := Generate(src, "client.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "M.new") {
		t.Errorf("kind %q missing 'M.new'", kind)
	}
	if !strings.Contains(kind, "M.fetch") {
		t.Errorf("kind %q missing 'M.fetch'", kind)
	}
}

// TestGenerate_Lua_MethodColon: function M:name() OOP style.
func TestGenerate_Lua_MethodColon(t *testing.T) {
	src := `
local Object = {}
Object.__index = Object

function Object:new(x, y)
  local o = {x=x, y=y}
  setmetatable(o, self)
  return o
end

function Object:move(dx, dy)
  self.x = self.x + dx
  self.y = self.y + dy
end
`
	fm, ok := Generate(src, "object.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Object:new") {
		t.Errorf("kind %q missing 'Object:new'", kind)
	}
	if !strings.Contains(kind, "Object:move") {
		t.Errorf("kind %q missing 'Object:move'", kind)
	}
}

// TestGenerate_Lua_AssignedFunctions: name = function() style.
func TestGenerate_Lua_AssignedFunctions(t *testing.T) {
	src := `
local handler = {}

handler.on_connect = function(conn)
  print("connected", conn)
end

handler.on_data = function(conn, data)
  conn:send(data)
end
`
	fm, ok := Generate(src, "handler.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "handler.on_connect") {
		t.Errorf("kind %q missing 'handler.on_connect'", kind)
	}
	if !strings.Contains(kind, "handler.on_data") {
		t.Errorf("kind %q missing 'handler.on_data'", kind)
	}
}

// TestGenerate_Lua_NestedNotCaptured: functions inside functions stay at depth>0.
func TestGenerate_Lua_NestedNotCaptured(t *testing.T) {
	src := `
function outer()
  local function inner()
    return 1
  end
  return inner()
end
`
	fm, ok := Generate(src, "nested.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "outer") {
		t.Errorf("kind %q missing 'outer'", kind)
	}
	if strings.Contains(kind, "inner") {
		t.Errorf("kind %q should not contain 'inner' (nested function)", kind)
	}
}

// TestGenerate_Lua_TableFunctionNotCaptured: functions inside table constructors
// are at depth>0 due to { tracking.
func TestGenerate_Lua_TableFunctionNotCaptured(t *testing.T) {
	src := `
function setup()
  local M = {
    method = function(self)
      return self
    end,
  }
  return M
end
`
	fm, ok := Generate(src, "table.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "setup") {
		t.Errorf("kind %q missing 'setup'", kind)
	}
	if strings.Contains(kind, "method") {
		t.Errorf("kind %q should not contain 'method' (inside table inside function)", kind)
	}
}

// TestGenerate_Lua_ForLoopDepth: functions inside for loops are nested.
func TestGenerate_Lua_ForLoopDepth(t *testing.T) {
	src := `
function process(items)
  for _, v in ipairs(items) do
    local function helper(x)
      return x * 2
    end
    helper(v)
  end
end
`
	fm, ok := Generate(src, "loop.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "helper") {
		t.Errorf("kind %q should not contain nested 'helper'", kind)
	}
}

// TestGenerate_Lua_StringsWithKeywords: "end" and "function" inside strings don't
// affect depth.
func TestGenerate_Lua_StringsWithKeywords(t *testing.T) {
	src := `
function make_message()
  local s = "function end while do"
  local t = 'another end here'
  return s .. t
end

function other()
end
`
	fm, ok := Generate(src, "strings.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "make_message") {
		t.Errorf("kind %q missing 'make_message'", kind)
	}
	if !strings.Contains(kind, "other") {
		t.Errorf("kind %q missing 'other'", kind)
	}
}

// TestGenerate_Lua_LongComment: content inside --[[ ]] doesn't affect depth.
func TestGenerate_Lua_LongComment(t *testing.T) {
	src := `
--[[
function fake_inside_comment()
end
]]

function real()
  return 1
end
`
	fm, ok := Generate(src, "comments.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "fake_inside_comment") {
		t.Errorf("kind %q should not contain function from inside --[[ comment", kind)
	}
	if !strings.Contains(kind, "real") {
		t.Errorf("kind %q missing 'real'", kind)
	}
}

// TestGenerate_Lua_MethodOverflow: more than 4 functions → (+N) suffix.
func TestGenerate_Lua_MethodOverflow(t *testing.T) {
	src := `
function a() end
function b() end
function c() end
function d() end
function e() end
function f() end
`
	fm, ok := Generate(src, "many.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+2)") {
		t.Errorf("kind %q expected '(+2)' overflow", kind)
	}
}

// TestGenerate_Lua_LineRange: map entry covers the whole file.
func TestGenerate_Lua_LineRange(t *testing.T) {
	src := `function foo()
  return 1
end
`
	fm, ok := Generate(src, "range.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	entry := fm.Map[0]
	if entry.Lines != "1-3" {
		t.Errorf("expected Lines='1-3', got %q", entry.Lines)
	}
}

// TestGenerate_Lua_GeneratedFile: generated file still returns ok=true but
// summary contains "generated".
func TestGenerate_Lua_GeneratedFile(t *testing.T) {
	src := `-- Generated by protoc-gen-lua. DO NOT EDIT.

function new_msg()
  return {}
end
`
	fm, ok := Generate(src, "proto.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("summary %q missing 'generated'", fm.Summary)
	}
}

// TestGenerate_Lua_FallsThrough_Empty: empty file falls through.
func TestGenerate_Lua_FallsThrough_Empty(t *testing.T) {
	_, ok := Generate("", "empty.lua")
	if ok {
		t.Fatal("expected ok=false for empty file")
	}
}

// TestGenerate_Lua_FallsThrough_NoFunctions: file with no function definitions
// falls through (e.g. pure config tables).
func TestGenerate_Lua_FallsThrough_NoFunctions(t *testing.T) {
	src := `
-- just constants and a table
local VERSION = "1.0"
local config = {debug=true, port=8080}
`
	_, ok := Generate(src, "config.lua")
	if ok {
		t.Fatal("expected ok=false for file with no function definitions")
	}
}

// TestGenerate_Lua_Summary: summary includes stem and function count.
func TestGenerate_Lua_Summary(t *testing.T) {
	src := `
function init() end
function tick() end
function render() end
`
	fm, ok := Generate(src, "engine.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "engine") {
		t.Errorf("summary %q missing 'engine'", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "3 functions") {
		t.Errorf("summary %q missing '3 functions'", fm.Summary)
	}
}

// TestGenerate_Lua_Symbols: Symbols slice contains "module <stem>".
func TestGenerate_Lua_Symbols(t *testing.T) {
	src := `function foo() end`
	fm, ok := Generate(src, "mymod.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(fm.Symbols) == 0 || fm.Symbols[0] != "module mymod" {
		t.Errorf("expected Symbols[0]='module mymod', got %v", fm.Symbols)
	}
}

// TestGenerate_Lua_IfForWhileDepth: control-flow bodies don't capture nested fns.
func TestGenerate_Lua_IfForWhileDepth(t *testing.T) {
	src := `
function top()
  if true then
    local function inside_if() end
  end
  for i = 1, 10 do
    local function inside_for() end
  end
  while false do
    local function inside_while() end
  end
end
`
	fm, ok := Generate(src, "ctrl.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	for _, name := range []string{"inside_if", "inside_for", "inside_while"} {
		if strings.Contains(kind, name) {
			t.Errorf("kind %q should not contain nested %q", kind, name)
		}
	}
	if !strings.Contains(kind, "top") {
		t.Errorf("kind %q missing 'top'", kind)
	}
}

// TestGenerate_Lua_Deduplication: same function name defined twice (override)
// appears only once.
func TestGenerate_Lua_Deduplication(t *testing.T) {
	src := `
function setup()
  print("v1")
end

-- override
function setup()
  print("v2")
end
`
	fm, ok := Generate(src, "dup.lua")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	// count occurrences of "setup" in kind
	count := strings.Count(kind, "setup")
	if count != 1 {
		t.Errorf("expected 'setup' once in kind %q, found %d times", kind, count)
	}
}
