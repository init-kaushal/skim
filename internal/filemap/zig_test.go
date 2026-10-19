package filemap

import (
	"strings"
	"testing"
)

// TestGenerate_Zig_Basic: typical Zig source with types and functions.
func TestGenerate_Zig_Basic(t *testing.T) {
	src := `const std = @import("std");

pub const Point = struct {
    x: f64,
    y: f64,

    pub fn distance(self: Point, other: Point) f64 {
        const dx = self.x - other.x;
        const dy = self.y - other.y;
        return std.math.sqrt(dx * dx + dy * dy);
    }
};

pub fn main() !void {
    const p1 = Point{ .x = 0, .y = 0 };
    const p2 = Point{ .x = 3, .y = 4 };
    std.debug.print("d={d}\n", .{p1.distance(p2)});
}
`
	fm, ok := Generate(src, "main.zig")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "Zig") {
		t.Errorf("summary %q missing Zig", fm.Summary)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Point") {
		t.Errorf("kind %q missing Point", kind)
	}
	if !strings.Contains(kind, "main") {
		t.Errorf("kind %q missing main", kind)
	}
	// Symbols must include type prefix for Point and fn prefix for main.
	hasTypePoint := false
	hasFnMain := false
	for _, s := range fm.Symbols {
		if s == "type Point" {
			hasTypePoint = true
		}
		if s == "fn main" {
			hasFnMain = true
		}
	}
	if !hasTypePoint {
		t.Errorf("symbols %v missing 'type Point'", fm.Symbols)
	}
	if !hasFnMain {
		t.Errorf("symbols %v missing 'fn main'", fm.Symbols)
	}
}

// TestGenerate_Zig_StructMethodNotCaptured: methods inside struct bodies are
// indented and must not appear as top-level symbols.
func TestGenerate_Zig_StructMethodNotCaptured(t *testing.T) {
	src := `pub const Allocator = struct {
    pub fn alloc(self: *Allocator, n: usize) ![]u8 {
        _ = self;
        _ = n;
        return error.OutOfMemory;
    }
};

pub fn newAllocator() Allocator {
    return .{};
}
`
	fm, ok := Generate(src, "alloc.zig")
	if !ok {
		t.Fatal("expected ok=true")
	}
	for _, s := range fm.Symbols {
		if s == "fn alloc" {
			t.Errorf("symbols should not contain indented method 'fn alloc', got %v", fm.Symbols)
		}
	}
	found := false
	for _, s := range fm.Symbols {
		if s == "fn newAllocator" {
			found = true
		}
	}
	if !found {
		t.Errorf("symbols %v missing 'fn newAllocator'", fm.Symbols)
	}
}

// TestGenerate_Zig_UnionEnum: tagged union and enum type declarations.
func TestGenerate_Zig_UnionEnum(t *testing.T) {
	src := `pub const Value = union(enum) {
    int: i64,
    float: f64,
    boolean: bool,
};

pub const Color = enum {
    red,
    green,
    blue,
};

pub fn describe(v: Value) void {
    _ = v;
}
`
	fm, ok := Generate(src, "types.zig")
	if !ok {
		t.Fatal("expected ok=true")
	}
	hasValue := false
	hasColor := false
	for _, s := range fm.Symbols {
		if s == "type Value" {
			hasValue = true
		}
		if s == "type Color" {
			hasColor = true
		}
	}
	if !hasValue {
		t.Errorf("symbols %v missing 'type Value'", fm.Symbols)
	}
	if !hasColor {
		t.Errorf("symbols %v missing 'type Color'", fm.Symbols)
	}
}

// TestGenerate_Zig_PackedExternStruct: packed and extern struct qualifiers.
func TestGenerate_Zig_PackedExternStruct(t *testing.T) {
	src := `pub const Header = packed struct {
    version: u8,
    flags: u8,
    length: u16,
};

pub const CStruct = extern struct {
    x: c_int,
    y: c_int,
};
`
	fm, ok := Generate(src, "ffi.zig")
	if !ok {
		t.Fatal("expected ok=true")
	}
	hasHeader := false
	hasCStruct := false
	for _, s := range fm.Symbols {
		if s == "type Header" {
			hasHeader = true
		}
		if s == "type CStruct" {
			hasCStruct = true
		}
	}
	if !hasHeader {
		t.Errorf("symbols %v missing 'type Header' (packed struct)", fm.Symbols)
	}
	if !hasCStruct {
		t.Errorf("symbols %v missing 'type CStruct' (extern struct)", fm.Symbols)
	}
}

// TestGenerate_Zig_FunctionModifiers: export, extern, inline, noinline fns.
func TestGenerate_Zig_FunctionModifiers(t *testing.T) {
	src := `pub export fn addC(a: c_int, b: c_int) c_int {
    return a + b;
}

pub inline fn fastAdd(a: u32, b: u32) u32 {
    return a + b;
}

pub noinline fn slowFn() void {}

fn privateFn() void {}
`
	fm, ok := Generate(src, "ffi.zig")
	if !ok {
		t.Fatal("expected ok=true")
	}
	want := []string{"fn addC", "fn fastAdd", "fn slowFn", "fn privateFn"}
	for _, w := range want {
		found := false
		for _, s := range fm.Symbols {
			if s == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("symbols %v missing %q", fm.Symbols, w)
		}
	}
}

// TestGenerate_Zig_CommentSkip: // comments are skipped.
func TestGenerate_Zig_CommentSkip(t *testing.T) {
	src := `// pub fn fakeFunc() void {}
// pub const FakeType = struct {};

pub fn real() void {}
`
	fm, ok := Generate(src, "commented.zig")
	if !ok {
		t.Fatal("expected ok=true")
	}
	for _, s := range fm.Symbols {
		if strings.Contains(s, "fake") || strings.Contains(s, "Fake") {
			t.Errorf("commented-out declarations must not appear in symbols, got %v", fm.Symbols)
		}
	}
	found := false
	for _, s := range fm.Symbols {
		if s == "fn real" {
			found = true
		}
	}
	if !found {
		t.Errorf("symbols %v missing 'fn real'", fm.Symbols)
	}
}

// TestGenerate_Zig_IndentedLocalConst: local const inside a function body
// must not be captured even if it happens to match the type regex pattern.
func TestGenerate_Zig_IndentedLocalConst(t *testing.T) {
	src := `pub fn setup() void {
    const LocalHelper = struct {
        x: u32,
    };
    _ = LocalHelper{};
}

pub const TopLevel = struct { y: u32 };
`
	fm, ok := Generate(src, "local.zig")
	if !ok {
		t.Fatal("expected ok=true")
	}
	for _, s := range fm.Symbols {
		if s == "type LocalHelper" {
			t.Errorf("indented local const struct must not appear in symbols, got %v", fm.Symbols)
		}
	}
	found := false
	for _, s := range fm.Symbols {
		if s == "type TopLevel" {
			found = true
		}
	}
	if !found {
		t.Errorf("symbols %v missing 'type TopLevel'", fm.Symbols)
	}
}

// TestGenerate_Zig_OpaqueType: opaque type declarations.
func TestGenerate_Zig_OpaqueType(t *testing.T) {
	src := `pub const Handle = opaque {};

pub fn open() *Handle {
    return undefined;
}
`
	fm, ok := Generate(src, "handle.zig")
	if !ok {
		t.Fatal("expected ok=true")
	}
	found := false
	for _, s := range fm.Symbols {
		if s == "type Handle" {
			found = true
		}
	}
	if !found {
		t.Errorf("symbols %v missing 'type Handle' (opaque)", fm.Symbols)
	}
}

// TestGenerate_Zig_OverflowLabel: more than 4 items uses +N notation.
func TestGenerate_Zig_OverflowLabel(t *testing.T) {
	src := `pub fn fn1() void {}
pub fn fn2() void {}
pub fn fn3() void {}
pub fn fn4() void {}
pub fn fn5() void {}
`
	fm, ok := Generate(src, "many.zig")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Map[0].Kind, "(+") {
		t.Errorf("kind %q should have overflow notation", fm.Map[0].Kind)
	}
}

// TestGenerate_Zig_Generated: generated file note in summary.
func TestGenerate_Zig_Generated(t *testing.T) {
	src := `// DO NOT EDIT — generated by build.zig
pub fn stub() void {}
`
	fm, ok := Generate(src, "generated.zig")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("summary %q missing generated note", fm.Summary)
	}
}

// TestGenerate_Zig_FallsThrough_Empty: empty file falls through.
func TestGenerate_Zig_FallsThrough_Empty(t *testing.T) {
	_, ok := Generate("", "empty.zig")
	if ok {
		t.Error("expected ok=false for empty file")
	}
}

// TestGenerate_Zig_FallsThrough_ImportsOnly: a file with only @import
// constants has no structural declarations.
func TestGenerate_Zig_FallsThrough_ImportsOnly(t *testing.T) {
	src := `const std = @import("std");
const builtin = @import("builtin");
`
	_, ok := Generate(src, "imports.zig")
	if ok {
		t.Error("expected ok=false for imports-only file")
	}
}

// TestGenerate_Zig_StemAsLabel: file stem is used as the label base (Zig has
// no module declaration).
func TestGenerate_Zig_StemAsLabel(t *testing.T) {
	src := `pub fn hello() void {}
`
	fm, ok := Generate(src, "my_module.zig")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Map[0].Kind, "my_module") {
		t.Errorf("kind %q should use file stem as label base", fm.Map[0].Kind)
	}
}

// TestGenerate_Zig_LineRange: single map entry covers the whole file.
func TestGenerate_Zig_LineRange(t *testing.T) {
	src := "pub fn hello() void {}\n"
	fm, ok := Generate(src, "m.zig")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 map entry, got %d", len(fm.Map))
	}
	if fm.Map[0].Lines != "1" {
		t.Errorf("Lines = %q, want 1", fm.Map[0].Lines)
	}
}
