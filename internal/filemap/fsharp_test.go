package filemap

import (
	"strings"
	"testing"
)

// TestGenerate_FSharp_Basic: a typical .fs file with module, types, and let bindings.
func TestGenerate_FSharp_Basic(t *testing.T) {
	src := `module MyApp.Core

type Point = { X: float; Y: float }
type Color = Red | Green | Blue

let distance (a: Point) (b: Point) =
    let dx = a.X - b.X
    let dy = a.Y - b.Y
    sqrt (dx*dx + dy*dy)

let origin = { X = 0.0; Y = 0.0 }
`
	fm, ok := Generate(src, "Core.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "F#") {
		t.Errorf("summary %q missing F#", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "MyApp.Core") {
		t.Errorf("summary %q missing module name", fm.Summary)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Point") {
		t.Errorf("kind %q missing Point", kind)
	}
	if !strings.Contains(kind, "Color") {
		t.Errorf("kind %q missing Color", kind)
	}
	if !strings.Contains(kind, "distance") {
		t.Errorf("kind %q missing distance", kind)
	}
	// Symbols must include type and fn prefixes.
	hasTypePoint := false
	hasFnDistance := false
	for _, s := range fm.Symbols {
		if s == "type Point" {
			hasTypePoint = true
		}
		if s == "fn distance" {
			hasFnDistance = true
		}
	}
	if !hasTypePoint {
		t.Errorf("symbols %v missing 'type Point'", fm.Symbols)
	}
	if !hasFnDistance {
		t.Errorf("symbols %v missing 'fn distance'", fm.Symbols)
	}
}

// TestGenerate_FSharp_Namespace: file with namespace + module.
func TestGenerate_FSharp_Namespace(t *testing.T) {
	src := `namespace MyCompany.Services

module UserService =
    let getUser id = id

module OrderService =
    let getOrder id = id
`
	fm, ok := Generate(src, "services.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	// Namespace captured as the label base.
	if !strings.Contains(fm.Summary, "MyCompany.Services") {
		t.Errorf("summary %q missing namespace name", fm.Summary)
	}
}

// TestGenerate_FSharp_Namespace_NoModule: namespace without a module.
func TestGenerate_FSharp_Namespace_NoModule(t *testing.T) {
	src := `namespace MyCompany.Models

type Config = {
    Host: string
    Port: int
}

let defaultConfig = { Host = "localhost"; Port = 8080 }
`
	fm, ok := Generate(src, "models.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "MyCompany.Models") {
		t.Errorf("summary %q missing namespace name", fm.Summary)
	}
	if !strings.Contains(fm.Map[0].Kind, "Config") {
		t.Errorf("kind %q missing Config", fm.Map[0].Kind)
	}
}

// TestGenerate_FSharp_Script: .fsx script file with let bindings.
func TestGenerate_FSharp_Script(t *testing.T) {
	src := `module Script

let greet name = printfn "Hello, %s!" name
let numbers = [1; 2; 3; 4; 5]
`
	fm, ok := Generate(src, "script.fsx")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Map[0].Kind, "greet") {
		t.Errorf("kind %q missing greet", fm.Map[0].Kind)
	}
}

// TestGenerate_FSharp_Signature: .fsi signature file with val declarations.
func TestGenerate_FSharp_Signature(t *testing.T) {
	src := `module MyLib.Math

type Vector = { X: float; Y: float; Z: float }

val add : Vector -> Vector -> Vector
val scale : float -> Vector -> Vector
val magnitude : Vector -> float
`
	fm, ok := Generate(src, "Math.fsi")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "F# signature") {
		t.Errorf("summary %q missing 'F# signature'", fm.Summary)
	}
	hasValAdd := false
	for _, s := range fm.Symbols {
		if s == "val add" {
			hasValAdd = true
		}
	}
	if !hasValAdd {
		t.Errorf("symbols %v missing 'val add'", fm.Symbols)
	}
}

// TestGenerate_FSharp_LetRec: let rec is captured correctly.
func TestGenerate_FSharp_LetRec(t *testing.T) {
	src := `module Recursion

let rec factorial n =
    if n <= 1 then 1
    else n * factorial (n - 1)

let rec fib n =
    match n with
    | 0 -> 0
    | 1 -> 1
    | _ -> fib (n-1) + fib (n-2)
`
	fm, ok := Generate(src, "Recursion.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "factorial") {
		t.Errorf("kind %q missing factorial", kind)
	}
	if !strings.Contains(kind, "fib") {
		t.Errorf("kind %q missing fib", kind)
	}
}

// TestGenerate_FSharp_AccessModifiers: private and inline modifiers do not corrupt the name.
func TestGenerate_FSharp_AccessModifiers(t *testing.T) {
	src := `module Modifiers

let private helper x = x + 1
let inline add a b = a + b
let mutable counter = 0
`
	fm, ok := Generate(src, "mod.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "helper") {
		t.Errorf("kind %q missing helper", kind)
	}
	if !strings.Contains(kind, "add") {
		t.Errorf("kind %q missing add", kind)
	}
	if !strings.Contains(kind, "counter") {
		t.Errorf("kind %q missing counter", kind)
	}
}

// TestGenerate_FSharp_BlockComment: declarations inside (* ... *) are ignored.
func TestGenerate_FSharp_BlockComment(t *testing.T) {
	src := `module Comments

(* This is a
   (* nested block comment *)
   let fakeDecl = 0
*)

let realFn x = x * 2
`
	fm, ok := Generate(src, "comments.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "fakeDecl") {
		t.Errorf("kind %q should not contain fakeDecl (inside block comment)", kind)
	}
	if !strings.Contains(kind, "realFn") {
		t.Errorf("kind %q missing realFn", kind)
	}
}

// TestGenerate_FSharp_LineComment: (* after // is not a comment opener.
func TestGenerate_FSharp_LineComment(t *testing.T) {
	src := `module LineComments

let value = 42 // comment (* not a block comment
let another = "hello"
`
	fm, ok := Generate(src, "lc.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "value") {
		t.Errorf("kind %q missing value", kind)
	}
	if !strings.Contains(kind, "another") {
		t.Errorf("kind %q missing another", kind)
	}
}

// TestGenerate_FSharp_IndentedSkip: methods inside type bodies are not captured.
func TestGenerate_FSharp_IndentedSkip(t *testing.T) {
	src := `module Shapes

type Circle(radius: float) =
    member _.Area() = System.Math.PI * radius * radius
    member _.Perimeter() = 2.0 * System.Math.PI * radius

let area (c: Circle) = c.Area()
`
	fm, ok := Generate(src, "shapes.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	// Indented members should not appear.
	if strings.Contains(kind, "Area") {
		t.Errorf("kind %q should not contain indented member Area", kind)
	}
	// Column-0 let binding should appear.
	if !strings.Contains(kind, "area") {
		t.Errorf("kind %q missing area", kind)
	}
	// Type should appear.
	if !strings.Contains(kind, "Circle") {
		t.Errorf("kind %q missing Circle", kind)
	}
}

// TestGenerate_FSharp_TypeVariants: various type declaration forms.
func TestGenerate_FSharp_TypeVariants(t *testing.T) {
	src := `module Types

type Color = Red | Green | Blue
type 'a Tree = Leaf | Node of 'a * 'a Tree * 'a Tree
type private Secret = { Token: string }
type alias = int
`
	fm, ok := Generate(src, "types.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Color") {
		t.Errorf("kind %q missing Color", kind)
	}
	if !strings.Contains(kind, "Tree") {
		t.Errorf("kind %q missing Tree", kind)
	}
	if !strings.Contains(kind, "Secret") {
		t.Errorf("kind %q missing Secret", kind)
	}
}

// TestGenerate_FSharp_OverflowLabel: more than 4 declarations use +N overflow.
func TestGenerate_FSharp_OverflowLabel(t *testing.T) {
	src := `module Many

let fn1 x = x
let fn2 x = x
let fn3 x = x
let fn4 x = x
let fn5 x = x
let fn6 x = x
`
	fm, ok := Generate(src, "many.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+") {
		t.Errorf("kind %q should have overflow notation", kind)
	}
}

// TestGenerate_FSharp_Generated: generated file note in summary.
func TestGenerate_FSharp_Generated(t *testing.T) {
	src := `// DO NOT EDIT — generated by codegen.fsx
module Generated

let stub x = x
`
	fm, ok := Generate(src, "generated.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("summary %q missing generated note", fm.Summary)
	}
}

// TestGenerate_FSharp_TripleStringIgnored: declarations inside a triple-quoted
// string must not appear in the map; real declarations after the string must.
//
// Regression for the missing triple-string state tracking. Without the fix,
// `let fakeFunction` and `type FakeType` (both at column 0 inside """) are
// matched by the declaration regexes and collected incorrectly.
func TestGenerate_FSharp_TripleStringIgnored(t *testing.T) {
	src := `module Example

let documentation = """
let fakeFunction = 42
type FakeType = { X: int }
"""

let realFunction = 1
`
	fm, ok := Generate(src, "Example.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	for _, sym := range fm.Symbols {
		if strings.Contains(sym, "fakeFunction") {
			t.Errorf("symbols %v must not contain fakeFunction (inside triple string)", fm.Symbols)
		}
		if strings.Contains(sym, "FakeType") {
			t.Errorf("symbols %v must not contain FakeType (inside triple string)", fm.Symbols)
		}
	}
	found := false
	for _, sym := range fm.Symbols {
		if sym == "fn realFunction" {
			found = true
		}
	}
	if !found {
		t.Errorf("symbols %v must contain fn realFunction (after triple string)", fm.Symbols)
	}
}

// TestGenerate_FSharp_TripleStringSameLine: opening and closing """ on the
// same line (even count) should not change string state.
func TestGenerate_FSharp_TripleStringSameLine(t *testing.T) {
	src := `module Inline

let x = """hello"""
let y = 2
`
	fm, ok := Generate(src, "inline.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	found := false
	for _, sym := range fm.Symbols {
		if sym == "fn y" {
			found = true
		}
	}
	if !found {
		t.Errorf("symbols %v missing fn y — single-line triple string incorrectly left state open", fm.Symbols)
	}
}

// TestGenerate_FSharp_FallsThrough_Empty: empty file falls through.
func TestGenerate_FSharp_FallsThrough_Empty(t *testing.T) {
	_, ok := Generate("", "empty.fs")
	if ok {
		t.Error("expected ok=false for empty file")
	}
}

// TestGenerate_FSharp_FallsThrough_NoStructure: plain text has no F# structure.
func TestGenerate_FSharp_FallsThrough_NoStructure(t *testing.T) {
	_, ok := Generate("just some text without declarations", "plain.fs")
	if ok {
		t.Error("expected ok=false for plain text")
	}
}

// TestGenerate_FSharp_ValOnlyFallsThrough: .fsi with only val is captured.
func TestGenerate_FSharp_ValOnly(t *testing.T) {
	src := `val myFunction : int -> int -> int
`
	fm, ok := Generate(src, "sig.fsi")
	if !ok {
		t.Fatal("expected ok=true for .fsi with val")
	}
	if !strings.Contains(fm.Map[0].Kind, "myFunction") {
		t.Errorf("kind %q missing myFunction", fm.Map[0].Kind)
	}
}

// TestGenerate_FSharp_WildcardNotCaptured: let _ = ... and let () = ... are not captured.
func TestGenerate_FSharp_WildcardNotCaptured(t *testing.T) {
	src := `module Wild

let _ = printfn "side effect"
let realFn x = x + 1
`
	fm, ok := Generate(src, "wild.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	for _, sym := range fm.Symbols {
		if sym == "fn _" {
			t.Errorf("symbols should not contain wildcard 'fn _'")
		}
	}
	found := false
	for _, sym := range fm.Symbols {
		if sym == "fn realFn" {
			found = true
		}
	}
	if !found {
		t.Errorf("symbols %v missing 'fn realFn'", fm.Symbols)
	}
}

// TestGenerate_FSharp_LineRange: single-entry map covers the whole file.
func TestGenerate_FSharp_LineRange(t *testing.T) {
	src := "module M\n\nlet x = 1\n"
	fm, ok := Generate(src, "m.fs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 map entry, got %d", len(fm.Map))
	}
	// 3 lines; single-entry range is "1-3".
	if fm.Map[0].Lines != "1-3" {
		t.Errorf("Lines = %q, want 1-3", fm.Map[0].Lines)
	}
}
