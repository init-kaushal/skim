package filemap

import (
	"strings"
	"testing"
)

// TestGenerate_Clojure_BasicFunctions: ns declaration and defn forms.
func TestGenerate_Clojure_BasicFunctions(t *testing.T) {
	src := `
(ns myapp.core
  (:require [clojure.string :as str]))

(defn greet [name]
  (str "Hello, " name "!"))

(defn farewell [name]
  (str "Goodbye, " name "."))
`
	fm, ok := Generate(src, "core.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	// Label uses ns name as module identifier.
	if !strings.Contains(kind, "module myapp.core") {
		t.Errorf("kind %q missing 'module myapp.core'", kind)
	}
	for _, name := range []string{"greet", "farewell"} {
		if !strings.Contains(kind, name) {
			t.Errorf("kind %q missing %q", kind, name)
		}
	}
}

// TestGenerate_Clojure_PrivateDefn: defn- (private function) captured.
func TestGenerate_Clojure_PrivateDefn(t *testing.T) {
	src := `
(ns myapp.util)

(defn- internal-helper [x]
  (* x 2))

(defn public-fn [x]
  (internal-helper x))
`
	fm, ok := Generate(src, "util.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "internal-helper") {
		t.Errorf("kind %q missing 'internal-helper'", kind)
	}
	if !strings.Contains(kind, "public-fn") {
		t.Errorf("kind %q missing 'public-fn'", kind)
	}
}

// TestGenerate_Clojure_Protocol: defprotocol captured as structural.
func TestGenerate_Clojure_Protocol(t *testing.T) {
	src := `
(ns myapp.protocols)

(defprotocol Drawable
  (draw [this ctx])
  (resize [this w h]))

(defprotocol Serializable
  (serialize [this])
  (deserialize [this data]))
`
	fm, ok := Generate(src, "protocols.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Drawable") {
		t.Errorf("kind %q missing 'Drawable'", kind)
	}
	if !strings.Contains(kind, "Serializable") {
		t.Errorf("kind %q missing 'Serializable'", kind)
	}
}

// TestGenerate_Clojure_Record: defrecord captured as structural.
func TestGenerate_Clojure_Record(t *testing.T) {
	src := `
(ns myapp.models)

(defrecord Point [x y])

(defrecord Circle [center radius])

(defn distance [p1 p2]
  (Math/sqrt (+ (Math/pow (- (:x p2) (:x p1)) 2)
                (Math/pow (- (:y p2) (:y p1)) 2))))
`
	fm, ok := Generate(src, "models.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Point") {
		t.Errorf("kind %q missing 'Point'", kind)
	}
	if !strings.Contains(kind, "Circle") {
		t.Errorf("kind %q missing 'Circle'", kind)
	}
	if !strings.Contains(kind, "distance") {
		t.Errorf("kind %q missing 'distance'", kind)
	}
}

// TestGenerate_Clojure_Defmacro: defmacro captured.
func TestGenerate_Clojure_Defmacro(t *testing.T) {
	src := "(ns myapp.macros)\n" +
		"(defmacro when-let* [bindings & body]\n" +
		"  (if (empty? bindings)\n" +
		"    `(do ~@body)\n" +
		"    `(when-let [~(first bindings) ~(second bindings)]\n" +
		"       ~@body)))\n"
	fm, ok := Generate(src, "macros.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "when-let*") {
		t.Errorf("kind %q missing 'when-let*'", kind)
	}
}

// TestGenerate_Clojure_Defmulti: defmulti captured as a def.
func TestGenerate_Clojure_Defmulti(t *testing.T) {
	src := `
(ns myapp.dispatch)

(defmulti area :shape)

(defmethod area :circle [{:keys [radius]}]
  (* Math/PI radius radius))

(defmethod area :square [{:keys [side]}]
  (* side side))
`
	fm, ok := Generate(src, "dispatch.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "area") {
		t.Errorf("kind %q missing 'area'", kind)
	}
	// defmethod is NOT captured (too verbose; defmulti is the structural declaration)
	// The map would show "area" once from defmulti, not repeated for each method.
}

// TestGenerate_Clojure_Def: def and defonce captured.
func TestGenerate_Clojure_Def(t *testing.T) {
	src := `
(ns myapp.config)

(def max-retries 3)

(defonce db-conn (atom nil))
`
	fm, ok := Generate(src, "config.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "max-retries") {
		t.Errorf("kind %q missing 'max-retries'", kind)
	}
	if !strings.Contains(kind, "db-conn") {
		t.Errorf("kind %q missing 'db-conn'", kind)
	}
}

// TestGenerate_Clojure_NestedNotCaptured: defn inside a let or another defn
// is at paren depth > 0 and must not appear in the map.
func TestGenerate_Clojure_NestedNotCaptured(t *testing.T) {
	src := `
(ns myapp.nested)

(defn outer [x]
  (let [inner (fn [y] (* y 2))]
    (inner x)))

(defn top-level [x]
  x)
`
	fm, ok := Generate(src, "nested.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "outer") {
		t.Errorf("kind %q missing 'outer'", kind)
	}
	if !strings.Contains(kind, "top-level") {
		t.Errorf("kind %q missing 'top-level'", kind)
	}
	// "inner" is inside the let, at depth > 0 — must not appear
	if strings.Contains(kind, "inner") {
		t.Errorf("kind %q should not contain 'inner' (nested fn)", kind)
	}
}

// TestGenerate_Clojure_StringParens: parens inside strings must not affect
// depth — "(defn fake []" inside a string must not be captured.
func TestGenerate_Clojure_StringParens(t *testing.T) {
	src := `
(ns myapp.strings)

(def doc-string "This is (a string with) parens and (more)")

(defn real-fn [x]
  (str "result: " x))
`
	fm, ok := Generate(src, "strings.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "doc-string") {
		t.Errorf("kind %q missing 'doc-string'", kind)
	}
	if !strings.Contains(kind, "real-fn") {
		t.Errorf("kind %q missing 'real-fn'", kind)
	}
}

// TestGenerate_Clojure_CommentLine: ; line comments are skipped entirely.
func TestGenerate_Clojure_CommentLine(t *testing.T) {
	src := `
(ns myapp.comments)

; (defn hidden [] "not real")

(defn visible [x]
  x)
`
	fm, ok := Generate(src, "comments.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "hidden") {
		t.Errorf("kind %q should not contain 'hidden' (commented out)", kind)
	}
	if !strings.Contains(kind, "visible") {
		t.Errorf("kind %q missing 'visible'", kind)
	}
}

// TestGenerate_Clojure_MultilineBody: a defn whose body spans many lines
// is still captured, and declarations after it are also captured.
func TestGenerate_Clojure_MultilineBody(t *testing.T) {
	src := `
(ns myapp.multiline)

(defn complex-fn
  "Docstring."
  [x y z]
  (let [a (* x y)
        b (+ a z)]
    (if (> b 0)
      (str "positive: " b)
      (str "non-positive: " b))))

(defn after-fn [x]
  x)
`
	fm, ok := Generate(src, "multiline.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "complex-fn") {
		t.Errorf("kind %q missing 'complex-fn'", kind)
	}
	if !strings.Contains(kind, "after-fn") {
		t.Errorf("kind %q missing 'after-fn'", kind)
	}
}

// TestGenerate_Clojure_NoNS: file without (ns ...) falls back to stem.
func TestGenerate_Clojure_NoNS(t *testing.T) {
	src := `
(defn helper [x]
  (* x 2))
`
	fm, ok := Generate(src, "helper.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	// Falls back to stem when no ns declaration
	if !strings.Contains(kind, "module helper") {
		t.Errorf("kind %q missing 'module helper'", kind)
	}
	if !strings.Contains(kind, "helper") {
		t.Errorf("kind %q missing function name 'helper'", kind)
	}
}

// TestGenerate_Clojure_ClojureScript: .cljs summary says "ClojureScript".
func TestGenerate_Clojure_ClojureScript(t *testing.T) {
	src := `
(ns myapp.ui
  (:require [reagent.core :as r]))

(defn root-component []
  [:div "Hello"])
`
	fm, ok := Generate(src, "ui.cljs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "ClojureScript") {
		t.Errorf("summary %q missing 'ClojureScript'", fm.Summary)
	}
}

// TestGenerate_Clojure_Overflow: more than 4 names produce (+N) suffix.
func TestGenerate_Clojure_Overflow(t *testing.T) {
	src := `
(ns myapp.many)
(defn a [] 1)
(defn b [] 2)
(defn c [] 3)
(defn d [] 4)
(defn e [] 5)
(defn f [] 6)
`
	fm, ok := Generate(src, "many.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+2)") {
		t.Errorf("kind %q expected '(+2)' overflow", kind)
	}
}

// TestGenerate_Clojure_Summary: summary includes ns name and counts.
func TestGenerate_Clojure_Summary(t *testing.T) {
	src := `
(ns myapp.core)
(defprotocol Foo)
(defn bar [] 1)
(defn baz [] 2)
`
	fm, ok := Generate(src, "core.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "myapp.core") {
		t.Errorf("summary %q missing 'myapp.core'", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "1 type") {
		t.Errorf("summary %q missing '1 type'", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "2 functions") {
		t.Errorf("summary %q missing '2 functions'", fm.Summary)
	}
}

// TestGenerate_Clojure_Symbols: structural items use "type", defs use "fn".
func TestGenerate_Clojure_Symbols(t *testing.T) {
	src := `
(ns myapp.sym)
(defrecord MyRecord [x])
(defn my-fn [y] y)
`
	fm, ok := Generate(src, "sym.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	var hasType, hasFn bool
	for _, sym := range fm.Symbols {
		if sym == "type MyRecord" {
			hasType = true
		}
		if sym == "fn my-fn" {
			hasFn = true
		}
	}
	if !hasType {
		t.Errorf("Symbols %v missing 'type MyRecord'", fm.Symbols)
	}
	if !hasFn {
		t.Errorf("Symbols %v missing 'fn my-fn'", fm.Symbols)
	}
}

// TestGenerate_Clojure_Generated: generated file summary includes "generated".
func TestGenerate_Clojure_Generated(t *testing.T) {
	src := `; Generated by codegen. DO NOT EDIT.
(ns myapp.gen)
(defn auto-fn [] nil)
`
	fm, ok := Generate(src, "gen.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("summary %q missing 'generated'", fm.Summary)
	}
}

// TestGenerate_Clojure_MultilineStringWithSemicolon: a multiline string whose
// closing quote is on a semicolon-prefixed line must not leave the scanner in
// string mode, causing subsequent declarations to be missed.
//
// Regression for the early-skip bug: blank/semicolon lines were skipped
// before clojureNetDepth ran, so a closing " on such a line was never seen.
func TestGenerate_Clojure_MultilineStringWithSemicolon(t *testing.T) {
	src := "(ns example.core)\n" +
		"\n" +
		"(def description \"first line\n" +
		"; this line is string content and closes the string: \"\n" +
		")\n" +
		"\n" +
		"(defn later [] 42)\n"
	fm, ok := Generate(src, "core.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "later") {
		t.Errorf("kind %q missing 'later' — multiline string with semicolon line corrupted depth", kind)
	}
}

// TestGenerate_Clojure_FallsThrough_Empty: empty file falls through.
func TestGenerate_Clojure_FallsThrough_Empty(t *testing.T) {
	_, ok := Generate("", "empty.clj")
	if ok {
		t.Fatal("expected ok=false for empty file")
	}
}

// TestGenerate_Clojure_FallsThrough_NoStructure: only require/import falls through.
func TestGenerate_Clojure_FallsThrough_NoStructure(t *testing.T) {
	src := `
; just some comments
; and no declarations
`
	_, ok := Generate(src, "nostructs.clj")
	if ok {
		t.Fatal("expected ok=false for file with no declarations")
	}
}

// TestGenerate_Clojure_NsOnlyActivates: a file with only (ns ...) should
// still produce ok=true (ns alone is useful structural info).
func TestGenerate_Clojure_NsOnlyActivates(t *testing.T) {
	src := `(ns myapp.empty)
`
	fm, ok := Generate(src, "empty.clj")
	if !ok {
		t.Fatal("expected ok=true — ns-only file should activate parser")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "myapp.empty") {
		t.Errorf("kind %q missing 'myapp.empty'", kind)
	}
}

// TestGenerate_Clojure_Deftype: deftype captured.
func TestGenerate_Clojure_Deftype(t *testing.T) {
	src := `
(ns myapp.types)

(deftype MyType [field1 field2]
  Serializable
  (serialize [this] (str field1 ":" field2)))
`
	fm, ok := Generate(src, "types.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "MyType") {
		t.Errorf("kind %q missing 'MyType'", kind)
	}
}

// TestGenerate_Clojure_ArrowName: Clojure symbols starting with -> captured.
func TestGenerate_Clojure_ArrowName(t *testing.T) {
	src := `
(ns myapp.transform)

(defn ->user-map [raw]
  {:name (:name raw)
   :email (:email raw)})
`
	fm, ok := Generate(src, "transform.clj")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "->user-map") {
		t.Errorf("kind %q missing '->user-map'", kind)
	}
}
