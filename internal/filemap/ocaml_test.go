package filemap

import (
	"strings"
	"testing"
)

// TestGenerate_OCaml_BasicModule: let bindings and type declarations.
func TestGenerate_OCaml_BasicModule(t *testing.T) {
	src := `
type config = {
  host : string;
  port : int;
}

let create_server cfg =
  Printf.printf "Starting on %s:%d\n" cfg.host cfg.port

let stop_server () =
  Printf.printf "Stopping\n"
`
	fm, ok := Generate(src, "server.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "module server") {
		t.Errorf("kind %q missing 'module server'", kind)
	}
	for _, name := range []string{"config", "create_server", "stop_server"} {
		if !strings.Contains(kind, name) {
			t.Errorf("kind %q missing %q", kind, name)
		}
	}
}

// TestGenerate_OCaml_LetRec: let rec captured correctly.
func TestGenerate_OCaml_LetRec(t *testing.T) {
	src := `
let rec factorial n =
  if n <= 1 then 1
  else n * factorial (n - 1)

let rec fib n =
  if n < 2 then n
  else fib (n-1) + fib (n-2)
`
	fm, ok := Generate(src, "math.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "factorial") {
		t.Errorf("kind %q missing 'factorial'", kind)
	}
	if !strings.Contains(kind, "fib") {
		t.Errorf("kind %q missing 'fib'", kind)
	}
}

// TestGenerate_OCaml_Types: various type declaration forms.
func TestGenerate_OCaml_Types(t *testing.T) {
	src := `
type color = Red | Green | Blue

type 'a option = None | Some of 'a

type nonrec point = { x : float; y : float }

exception Not_found_in_index
`
	fm, ok := Generate(src, "types.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	for _, name := range []string{"color", "option", "point", "Not_found_in_index"} {
		if !strings.Contains(kind, name) {
			t.Errorf("kind %q missing %q", kind, name)
		}
	}
}

// TestGenerate_OCaml_Modules: module declarations captured.
func TestGenerate_OCaml_Modules(t *testing.T) {
	src := `
module StringMap = Map.Make(String)

module type Hashable = sig
  type t
  val hash : t -> int
end
`
	fm, ok := Generate(src, "modules.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "StringMap") {
		t.Errorf("kind %q missing 'StringMap'", kind)
	}
	if !strings.Contains(kind, "Hashable") {
		t.Errorf("kind %q missing 'Hashable'", kind)
	}
}

// TestGenerate_OCaml_Interface: .mli file captures val declarations.
func TestGenerate_OCaml_Interface(t *testing.T) {
	src := `
type t

val create : string -> t

val process : t -> string -> unit

val to_string : t -> string
`
	fm, ok := Generate(src, "parser.mli")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "module parser") {
		t.Errorf("kind %q missing 'module parser'", kind)
	}
	for _, name := range []string{"t", "create", "process", "to_string"} {
		if !strings.Contains(kind, name) {
			t.Errorf("kind %q missing %q", kind, name)
		}
	}
}

// TestGenerate_OCaml_InterfaceSummary: .mli summary says "interface" and "val".
func TestGenerate_OCaml_InterfaceSummary(t *testing.T) {
	src := `
type t
val foo : t -> int
val bar : t -> string
`
	fm, ok := Generate(src, "mymod.mli")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "interface") {
		t.Errorf("summary %q missing 'interface'", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "2 vals") {
		t.Errorf("summary %q missing '2 vals'", fm.Summary)
	}
}

// TestGenerate_OCaml_IndentedLinesSkipped: let bindings and types inside
// struct / sig blocks are indented and must not appear in the map.
func TestGenerate_OCaml_IndentedLinesSkipped(t *testing.T) {
	src := `
module Inner = struct
  type hidden = int
  let inner_fn x = x + 1
end

let outer x = x * 2
`
	fm, ok := Generate(src, "scope.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "hidden") {
		t.Errorf("kind %q should not contain 'hidden' (inside struct)", kind)
	}
	if strings.Contains(kind, "inner_fn") {
		t.Errorf("kind %q should not contain 'inner_fn' (inside struct)", kind)
	}
	if !strings.Contains(kind, "Inner") {
		t.Errorf("kind %q missing 'Inner'", kind)
	}
	if !strings.Contains(kind, "outer") {
		t.Errorf("kind %q missing 'outer'", kind)
	}
}

// TestGenerate_OCaml_BlockComment: declarations inside (* *) are skipped.
func TestGenerate_OCaml_BlockComment(t *testing.T) {
	src := `
(* let hidden x = x + 1 *)

let visible x = x - 1

(* type also_hidden = int *)

type real_type = string
`
	fm, ok := Generate(src, "comments.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "hidden") {
		t.Errorf("kind %q should not contain 'hidden' (commented out)", kind)
	}
	if strings.Contains(kind, "also_hidden") {
		t.Errorf("kind %q should not contain 'also_hidden' (commented out)", kind)
	}
	if !strings.Contains(kind, "visible") {
		t.Errorf("kind %q missing 'visible'", kind)
	}
	if !strings.Contains(kind, "real_type") {
		t.Errorf("kind %q missing 'real_type'", kind)
	}
}

// TestGenerate_OCaml_NestedBlockComment: inner *) must not close the outer
// comment — same nesting semantics as Haskell's {- -}.
func TestGenerate_OCaml_NestedBlockComment(t *testing.T) {
	src := `
(*
  Outer comment.
  (* nested *) still in outer
let fake x = x
*)

let real x = x + 1
`
	fm, ok := Generate(src, "nested.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "fake") {
		t.Errorf("kind %q should not contain 'fake' from nested comment", kind)
	}
	if !strings.Contains(kind, "real") {
		t.Errorf("kind %q missing 'real'", kind)
	}
}

// TestGenerate_OCaml_WildcardSkipped: let _ = ... and let () = ... are not
// captured (no meaningful name).
func TestGenerate_OCaml_WildcardSkipped(t *testing.T) {
	src := `
let () = Printf.printf "init\n"

let _ = ignore 42

let named x = x + 1
`
	fm, ok := Generate(src, "init.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "named") {
		t.Errorf("kind %q missing 'named'", kind)
	}
}

// TestGenerate_OCaml_Overflow: more than 4 names get (+N) suffix.
func TestGenerate_OCaml_Overflow(t *testing.T) {
	src := `
let a x = x
let b x = x
let c x = x
let d x = x
let e x = x
let f x = x
`
	fm, ok := Generate(src, "many.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+2)") {
		t.Errorf("kind %q expected '(+2)' overflow", kind)
	}
}

// TestGenerate_OCaml_LineRange: map entry spans the whole file.
func TestGenerate_OCaml_LineRange(t *testing.T) {
	src := `let foo x = x + 1
`
	fm, ok := Generate(src, "range.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if fm.Map[0].Lines != "1" {
		t.Errorf("expected Lines='1', got %q", fm.Map[0].Lines)
	}
}

// TestGenerate_OCaml_GeneratedFile: generated summary includes "generated".
func TestGenerate_OCaml_GeneratedFile(t *testing.T) {
	src := `(* Generated by ocamllex. DO NOT EDIT. *)
let token buf = Lexing.next_token buf
`
	fm, ok := Generate(src, "lexer.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("summary %q missing 'generated'", fm.Summary)
	}
}

// TestGenerate_OCaml_Summary: summary includes stem and correct counts.
func TestGenerate_OCaml_Summary(t *testing.T) {
	src := `
type t = int
type s = string
let foo x = x
`
	fm, ok := Generate(src, "mymodule.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "mymodule") {
		t.Errorf("summary %q missing 'mymodule'", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "2 types") {
		t.Errorf("summary %q missing '2 types'", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "1 function") {
		t.Errorf("summary %q missing '1 function'", fm.Summary)
	}
}

// TestGenerate_OCaml_Symbols: structural items use "type" prefix, defs use "fn".
func TestGenerate_OCaml_Symbols(t *testing.T) {
	src := `
type my_type = int
let my_fn x = x
`
	fm, ok := Generate(src, "s.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	var hasType, hasFn bool
	for _, sym := range fm.Symbols {
		if sym == "type my_type" {
			hasType = true
		}
		if sym == "fn my_fn" {
			hasFn = true
		}
	}
	if !hasType {
		t.Errorf("Symbols %v missing 'type my_type'", fm.Symbols)
	}
	if !hasFn {
		t.Errorf("Symbols %v missing 'fn my_fn'", fm.Symbols)
	}
}

// TestGenerate_OCaml_ExceptionOnly: a file containing only an exception
// declaration must activate the parser (hasOCamlStructure was missing
// ocamlException).
func TestGenerate_OCaml_ExceptionOnly(t *testing.T) {
	src := `exception Invalid_input of string
`
	fm, ok := Generate(src, "errors.ml")
	if !ok {
		t.Fatal("expected ok=true — exception-only file must activate parser")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Invalid_input") {
		t.Errorf("kind %q missing 'Invalid_input'", kind)
	}
}

// TestGenerate_OCaml_FallsThrough_Empty: empty file falls through.
func TestGenerate_OCaml_FallsThrough_Empty(t *testing.T) {
	_, ok := Generate("", "empty.ml")
	if ok {
		t.Fatal("expected ok=false for empty file")
	}
}

// TestGenerate_OCaml_FallsThrough_NoStructure: only open/comments falls through.
func TestGenerate_OCaml_FallsThrough_NoStructure(t *testing.T) {
	src := `
open Printf
(* just a comment *)
`
	_, ok := Generate(src, "nostructs.ml")
	if ok {
		t.Fatal("expected ok=false for file with no declarations")
	}
}

// TestGenerate_OCaml_Class: class declarations captured.
func TestGenerate_OCaml_Class(t *testing.T) {
	src := `
class point x_init y_init = object
  val mutable x = x_init
  val mutable y = y_init
  method get_x () = x
  method get_y () = y
end

class virtual shape = object
  method virtual area : float
end
`
	fm, ok := Generate(src, "shapes.ml")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "point") {
		t.Errorf("kind %q missing 'point'", kind)
	}
	if !strings.Contains(kind, "shape") {
		t.Errorf("kind %q missing 'shape'", kind)
	}
}
