package filemap

import (
	"strings"
	"testing"
)

// TestGenerate_Julia_BasicFunctions: standard function declarations.
func TestGenerate_Julia_BasicFunctions(t *testing.T) {
	src := `
function add(x, y)
    return x + y
end

function subtract(x, y)
    return x - y
end

function multiply(x, y)
    return x * y
end
`
	fm, ok := Generate(src, "math.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "module math") {
		t.Errorf("kind %q missing 'module math'", kind)
	}
	for _, name := range []string{"add", "subtract", "multiply"} {
		if !strings.Contains(kind, name) {
			t.Errorf("kind %q missing %q", kind, name)
		}
	}
}

// TestGenerate_Julia_Structs: struct and mutable struct declarations.
func TestGenerate_Julia_Structs(t *testing.T) {
	src := `
struct Point
    x::Float64
    y::Float64
end

mutable struct Config
    name::String
    value::Int
end
`
	fm, ok := Generate(src, "types.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Point") {
		t.Errorf("kind %q missing 'Point'", kind)
	}
	if !strings.Contains(kind, "Config") {
		t.Errorf("kind %q missing 'Config'", kind)
	}
}

// TestGenerate_Julia_AbstractType: abstract type declarations.
func TestGenerate_Julia_AbstractType(t *testing.T) {
	src := `
abstract type Animal end

abstract type Vehicle end

struct Car <: Vehicle
    make::String
end
`
	fm, ok := Generate(src, "hierarchy.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Animal") {
		t.Errorf("kind %q missing 'Animal'", kind)
	}
	if !strings.Contains(kind, "Vehicle") {
		t.Errorf("kind %q missing 'Vehicle'", kind)
	}
	if !strings.Contains(kind, "Car") {
		t.Errorf("kind %q missing 'Car'", kind)
	}
}

// TestGenerate_Julia_Module: module declaration captured.
func TestGenerate_Julia_Module(t *testing.T) {
	src := `
module Statistics

function mean(xs)
    sum(xs) / length(xs)
end

end
`
	fm, ok := Generate(src, "stats.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Statistics") {
		t.Errorf("kind %q missing 'Statistics'", kind)
	}
	// function inside module is at depth > 0, must not appear
	if strings.Contains(kind, "mean") {
		t.Errorf("kind %q should not contain 'mean' (inside module block)", kind)
	}
}

// TestGenerate_Julia_Macro: macro declarations captured.
func TestGenerate_Julia_Macro(t *testing.T) {
	src := `
macro assert_equal(a, b)
    quote
        if $(esc(a)) != $(esc(b))
            error("assertion failed")
        end
    end
end
`
	fm, ok := Generate(src, "macros.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "assert_equal") {
		t.Errorf("kind %q missing 'assert_equal'", kind)
	}
}

// TestGenerate_Julia_Const: top-level const declarations captured.
func TestGenerate_Julia_Const(t *testing.T) {
	src := `
const MAX_ITER = 1000
const EPSILON = 1e-8

function converge(x)
    for _ in 1:MAX_ITER
        x = update(x)
    end
    x
end
`
	fm, ok := Generate(src, "solver.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "MAX_ITER") {
		t.Errorf("kind %q missing 'MAX_ITER'", kind)
	}
	if !strings.Contains(kind, "converge") {
		t.Errorf("kind %q missing 'converge'", kind)
	}
}

// TestGenerate_Julia_NestedNotCaptured: functions/structs inside a function
// body are at depth > 0 and must not appear in the map.
func TestGenerate_Julia_NestedNotCaptured(t *testing.T) {
	src := `
function outer(x)
    function inner(y)
        y * 2
    end
    inner(x) + 1
end
`
	fm, ok := Generate(src, "nested.jl")
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

// TestGenerate_Julia_ForLoopDepth: for loop increments depth; declarations
// inside must not be captured.
func TestGenerate_Julia_ForLoopDepth(t *testing.T) {
	src := `
function process(items)
    for item in items
        function transform(x)
            x + 1
        end
    end
end

function top_level()
    return 42
end
`
	fm, ok := Generate(src, "loops.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "transform") {
		t.Errorf("kind %q should not contain 'transform' (inside for loop)", kind)
	}
	if !strings.Contains(kind, "top_level") {
		t.Errorf("kind %q missing 'top_level'", kind)
	}
}

// TestGenerate_Julia_BracketEnd: `end` inside [] must not decrement block
// depth — it is a subscript shorthand, not a block terminator.
func TestGenerate_Julia_BracketEnd(t *testing.T) {
	src := `
function slice_last(arr)
    return arr[2:end]
end

function use_end_idx(m)
    return m[1:end, 1:end]
end
`
	fm, ok := Generate(src, "subscript.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "slice_last") {
		t.Errorf("kind %q missing 'slice_last'", kind)
	}
	if !strings.Contains(kind, "use_end_idx") {
		t.Errorf("kind %q missing 'use_end_idx'", kind)
	}
}

// TestGenerate_Julia_EndInString: `end` inside a string literal must not
// affect block depth.
func TestGenerate_Julia_EndInString(t *testing.T) {
	src := `
function greet()
    msg = "end of the world"
    println(msg)
end

function farewell()
    return "the end"
end
`
	fm, ok := Generate(src, "strings.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "greet") {
		t.Errorf("kind %q missing 'greet'", kind)
	}
	if !strings.Contains(kind, "farewell") {
		t.Errorf("kind %q missing 'farewell'", kind)
	}
}

// TestGenerate_Julia_TripleString: `end` inside a """...""" multi-line string
// must not affect block depth.
func TestGenerate_Julia_TripleString(t *testing.T) {
	src := `
function get_template()
    return """
    Hello, this is the end
    of the template string.
    """
end

function other()
    return 42
end
`
	fm, ok := Generate(src, "template.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "get_template") {
		t.Errorf("kind %q missing 'get_template'", kind)
	}
	if !strings.Contains(kind, "other") {
		t.Errorf("kind %q missing 'other'", kind)
	}
}

// TestGenerate_Julia_BlockComment: `end` and keywords inside #= ... =# block
// comments are skipped.
func TestGenerate_Julia_BlockComment(t *testing.T) {
	src := `
#=
function hidden()
    return 1
end
=#

function visible()
    return 2
end
`
	fm, ok := Generate(src, "blockcomment.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "hidden") {
		t.Errorf("kind %q should not contain 'hidden' (in block comment)", kind)
	}
	if !strings.Contains(kind, "visible") {
		t.Errorf("kind %q missing 'visible'", kind)
	}
}

// TestGenerate_Julia_NestedBlockComment: inner =# must not close the outer
// block comment.
func TestGenerate_Julia_NestedBlockComment(t *testing.T) {
	src := `
#=
outer comment
#= nested =# still in outer
function fake()
end
=#

function real_fn()
    return 1
end
`
	fm, ok := Generate(src, "nestedcomment.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "fake") {
		t.Errorf("kind %q should not contain 'fake' (nested block comment)", kind)
	}
	if !strings.Contains(kind, "real_fn") {
		t.Errorf("kind %q missing 'real_fn'", kind)
	}
}

// TestGenerate_Julia_InlineEndDoesNotLeak: a single-line construct like
// `abstract type Foo end` must not leave depth < 0 (opener and closer
// cancel out on the same line).
func TestGenerate_Julia_InlineEndDoesNotLeak(t *testing.T) {
	src := `
abstract type Shape end

abstract type Color end

function area(s::Shape)
    return 0.0
end
`
	fm, ok := Generate(src, "shapes.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Shape") {
		t.Errorf("kind %q missing 'Shape'", kind)
	}
	if !strings.Contains(kind, "Color") {
		t.Errorf("kind %q missing 'Color'", kind)
	}
	if !strings.Contains(kind, "area") {
		t.Errorf("kind %q missing 'area'", kind)
	}
}

// TestGenerate_Julia_QualifiedFunction: Base.show style qualified names.
func TestGenerate_Julia_QualifiedFunction(t *testing.T) {
	src := `
struct MyType
    val::Int
end

function Base.show(io::IO, x::MyType)
    print(io, "MyType(", x.val, ")")
end
`
	fm, ok := Generate(src, "showext.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "MyType") {
		t.Errorf("kind %q missing 'MyType'", kind)
	}
	// Qualified names should capture at least the qualified form
	if !strings.Contains(kind, "Base.show") {
		t.Errorf("kind %q missing 'Base.show'", kind)
	}
}

// TestGenerate_Julia_Overflow: more than 4 names produce (+N) suffix.
func TestGenerate_Julia_Overflow(t *testing.T) {
	src := `
function a() end
function b() end
function c() end
function d() end
function e() end
function f() end
`
	fm, ok := Generate(src, "many.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+2)") {
		t.Errorf("kind %q expected '(+2)' overflow", kind)
	}
}

// TestGenerate_Julia_LineRange: single-function file.
func TestGenerate_Julia_LineRange(t *testing.T) {
	src := `function foo(x)
    x + 1
end
`
	fm, ok := Generate(src, "range.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if fm.Map[0].Lines != "1-3" {
		t.Errorf("expected Lines='1-3', got %q", fm.Map[0].Lines)
	}
}

// TestGenerate_Julia_Generated: generated file summary includes "generated".
func TestGenerate_Julia_Generated(t *testing.T) {
	src := `# Generated by codegen. DO NOT EDIT.
function auto_fn()
    return 1
end
`
	fm, ok := Generate(src, "gen.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("summary %q missing 'generated'", fm.Summary)
	}
}

// TestGenerate_Julia_Summary: summary includes stem and counts.
func TestGenerate_Julia_Summary(t *testing.T) {
	src := `
struct Foo end
abstract type Bar end
function baz() end
`
	fm, ok := Generate(src, "mymod.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "mymod") {
		t.Errorf("summary %q missing 'mymod'", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "2 types") {
		t.Errorf("summary %q missing '2 types'", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "1 function") {
		t.Errorf("summary %q missing '1 function'", fm.Summary)
	}
}

// TestGenerate_Julia_Symbols: structural items use "type" prefix, defs "fn".
func TestGenerate_Julia_Symbols(t *testing.T) {
	src := `
struct MyStruct end
function my_fn() end
`
	fm, ok := Generate(src, "sym.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	var hasType, hasFn bool
	for _, sym := range fm.Symbols {
		if sym == "type MyStruct" {
			hasType = true
		}
		if sym == "fn my_fn" {
			hasFn = true
		}
	}
	if !hasType {
		t.Errorf("Symbols %v missing 'type MyStruct'", fm.Symbols)
	}
	if !hasFn {
		t.Errorf("Symbols %v missing 'fn my_fn'", fm.Symbols)
	}
}

// TestGenerate_Julia_CommandLiteral: keywords inside a backtick command
// literal must not affect block depth (regression for inBacktick omission).
func TestGenerate_Julia_CommandLiteral(t *testing.T) {
	src := `
function run_command()
    cmd = ` + "`" + `if command_exists` + "`" + `
    run(cmd)
end

function later()
    return 42
end
`
	fm, ok := Generate(src, "cmds.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "run_command") {
		t.Errorf("kind %q missing 'run_command'", kind)
	}
	if !strings.Contains(kind, "later") {
		t.Errorf("kind %q missing 'later' — command literal corrupted block depth", kind)
	}
}

// TestGenerate_Julia_PrimitiveType: primitive type declarations captured.
func TestGenerate_Julia_PrimitiveType(t *testing.T) {
	src := `
primitive type MyBits16 16 end

primitive type MyBits32 <: AbstractFloat 32 end

function use_prim(x::MyBits16)
    return x
end
`
	fm, ok := Generate(src, "primitive.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "MyBits16") {
		t.Errorf("kind %q missing 'MyBits16'", kind)
	}
	if !strings.Contains(kind, "MyBits32") {
		t.Errorf("kind %q missing 'MyBits32'", kind)
	}
	if !strings.Contains(kind, "use_prim") {
		t.Errorf("kind %q missing 'use_prim'", kind)
	}
}

// TestGenerate_Julia_FallsThrough_Empty: empty file falls through.
func TestGenerate_Julia_FallsThrough_Empty(t *testing.T) {
	_, ok := Generate("", "empty.jl")
	if ok {
		t.Fatal("expected ok=false for empty file")
	}
}

// TestGenerate_Julia_FallsThrough_NoStructure: file with only imports
// and comments falls through.
func TestGenerate_Julia_FallsThrough_NoStructure(t *testing.T) {
	src := `
using LinearAlgebra
import Statistics
# just setup
`
	_, ok := Generate(src, "setup.jl")
	if ok {
		t.Fatal("expected ok=false for file with no declarations")
	}
}

// TestGenerate_Julia_IfForDepth: if/for increment depth; declarations
// at those levels are not top-level.
func TestGenerate_Julia_IfForDepth(t *testing.T) {
	src := `
x = 5
if x > 0
    function pos_fn()
        return x
    end
end

function top_fn()
    return 1
end
`
	fm, ok := Generate(src, "control.jl")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "pos_fn") {
		t.Errorf("kind %q should not contain 'pos_fn' (inside if block)", kind)
	}
	if !strings.Contains(kind, "top_fn") {
		t.Errorf("kind %q missing 'top_fn'", kind)
	}
}
