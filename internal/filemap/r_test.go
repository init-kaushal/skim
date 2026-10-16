package filemap

import (
	"strings"
	"testing"
)

// TestGenerate_R_BasicFunctions: standard <- assignment form.
func TestGenerate_R_BasicFunctions(t *testing.T) {
	src := `
load_data <- function(path) {
  read.csv(path)
}

clean_data <- function(df) {
  na.omit(df)
}

plot_results <- function(df) {
  plot(df$x, df$y)
}
`
	fm, ok := Generate(src, "analysis.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "module analysis") {
		t.Errorf("kind %q missing 'module analysis'", kind)
	}
	for _, fn := range []string{"load_data", "clean_data", "plot_results"} {
		if !strings.Contains(kind, fn) {
			t.Errorf("kind %q missing %q", kind, fn)
		}
	}
}

// TestGenerate_R_EqualAssign: = assignment form at column 0.
func TestGenerate_R_EqualAssign(t *testing.T) {
	src := `
greet = function(name) {
  paste("Hello", name)
}
`
	fm, ok := Generate(src, "greet.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "greet") {
		t.Errorf("kind %q missing 'greet'", kind)
	}
}

// TestGenerate_R_NestedNotCaptured: functions defined inside other function
// bodies are at brace depth > 0 and must not be captured.
func TestGenerate_R_NestedNotCaptured(t *testing.T) {
	src := `
outer <- function(x) {
  inner <- function(y) {
    y * 2
  }
  inner(x) + 1
}
`
	fm, ok := Generate(src, "nested.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "outer") {
		t.Errorf("kind %q missing 'outer'", kind)
	}
	if strings.Contains(kind, "inner") {
		t.Errorf("kind %q should not contain nested 'inner'", kind)
	}
}

// TestGenerate_R_NamedArgNotCaptured: function defined as a named argument
// inside a call is at paren depth > 0 and must not appear in the map.
func TestGenerate_R_NamedArgNotCaptured(t *testing.T) {
	src := `
result <- lapply(1:10, function(x) x^2)

handlers <- list(
  on_data = function(x) process(x),
  on_error = function(e) stop(e)
)

top_level <- function(x) x + 1
`
	fm, ok := Generate(src, "args.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "on_data") {
		t.Errorf("kind %q should not contain 'on_data' (named argument)", kind)
	}
	if strings.Contains(kind, "on_error") {
		t.Errorf("kind %q should not contain 'on_error' (named argument)", kind)
	}
	if !strings.Contains(kind, "top_level") {
		t.Errorf("kind %q missing 'top_level'", kind)
	}
}

// TestGenerate_R_S4Class: setClass captures the class name.
func TestGenerate_R_S4Class(t *testing.T) {
	src := `
setClass("Animal", representation(
  name = "character",
  weight = "numeric"
))

setGeneric("speak", function(x) standardGeneric("speak"))

speak <- function(x) UseMethod("speak")
`
	fm, ok := Generate(src, "s4.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Animal") {
		t.Errorf("kind %q missing 'Animal'", kind)
	}
	if !strings.Contains(kind, "speak") {
		t.Errorf("kind %q missing 'speak'", kind)
	}
}

// TestGenerate_R_R6Class: R6Class assignment captures the variable name.
func TestGenerate_R_R6Class(t *testing.T) {
	src := `
library(R6)

Counter <- R6Class("Counter",
  public = list(
    count = 0,
    increment = function() {
      self$count <- self$count + 1
    }
  )
)

reset <- function(counter) {
  counter$count <- 0
}
`
	fm, ok := Generate(src, "r6.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Counter") {
		t.Errorf("kind %q missing 'Counter'", kind)
	}
	if !strings.Contains(kind, "reset") {
		t.Errorf("kind %q missing 'reset'", kind)
	}
}

// TestGenerate_R_SetRefClass: setRefClass captures via the variable name.
func TestGenerate_R_SetRefClass(t *testing.T) {
	src := `
Queue <- setRefClass("Queue",
  fields = list(items = "list"),
  methods = list(
    push = function(x) { items[[length(items)+1]] <<- x },
    pop  = function()  { items[[1]] }
  )
)
`
	fm, ok := Generate(src, "queue.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Queue") {
		t.Errorf("kind %q missing 'Queue'", kind)
	}
}

// TestGenerate_R_StringWithBraces: braces inside strings don't affect depth.
func TestGenerate_R_StringWithBraces(t *testing.T) {
	src := `
make_sql <- function(table) {
  paste0("SELECT * FROM {", table, "}")
}

query_db <- function(sql) {
  dbGetQuery(conn, sql)
}
`
	fm, ok := Generate(src, "db.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "make_sql") {
		t.Errorf("kind %q missing 'make_sql'", kind)
	}
	if !strings.Contains(kind, "query_db") {
		t.Errorf("kind %q missing 'query_db'", kind)
	}
}

// TestGenerate_R_CommentSkipped: names after # are not captured.
func TestGenerate_R_CommentSkipped(t *testing.T) {
	src := `
# hidden <- function(x) x

visible <- function(x) x + 1
`
	fm, ok := Generate(src, "comments.R")
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

// TestGenerate_R_MultilineFunctionHeader: function body opens on a later line.
func TestGenerate_R_MultilineFunctionHeader(t *testing.T) {
	src := `
complex_fn <- function(
  x,
  y,
  z = TRUE
) {
  x + y
}

simple <- function(x) x
`
	fm, ok := Generate(src, "multiline.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "complex_fn") {
		t.Errorf("kind %q missing 'complex_fn'", kind)
	}
	if !strings.Contains(kind, "simple") {
		t.Errorf("kind %q missing 'simple'", kind)
	}
}

// TestGenerate_R_Overflow: more than 4 names produce (+N) suffix.
func TestGenerate_R_Overflow(t *testing.T) {
	src := `
a <- function() 1
b <- function() 2
c <- function() 3
d <- function() 4
e <- function() 5
f <- function() 6
`
	fm, ok := Generate(src, "many.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+2)") {
		t.Errorf("kind %q expected '(+2)' overflow", kind)
	}
}

// TestGenerate_R_LineRange: map entry covers the whole file.
func TestGenerate_R_LineRange(t *testing.T) {
	src := `foo <- function(x) x
`
	fm, ok := Generate(src, "range.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if fm.Map[0].Lines != "1" {
		t.Errorf("expected Lines='1', got %q", fm.Map[0].Lines)
	}
}

// TestGenerate_R_GeneratedFile: generated files produce ok=true with
// "generated" in the summary.
func TestGenerate_R_GeneratedFile(t *testing.T) {
	src := `# Generated by RcppExports. DO NOT EDIT.
.call_foo <- function(x) {
  .Call("foo", x)
}
`
	fm, ok := Generate(src, "RcppExports.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("summary %q missing 'generated'", fm.Summary)
	}
}

// TestGenerate_R_Summary: summary includes stem and counts.
func TestGenerate_R_Summary(t *testing.T) {
	src := `
setClass("Foo", representation(x = "numeric"))

bar <- function() 1
baz <- function() 2
`
	fm, ok := Generate(src, "mymodule.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "mymodule") {
		t.Errorf("summary %q missing 'mymodule'", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "1 class") {
		t.Errorf("summary %q missing '1 class'", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "2 functions") {
		t.Errorf("summary %q missing '2 functions'", fm.Summary)
	}
}

// TestGenerate_R_Symbols: Symbols slice uses "class" and "fn" prefixes.
func TestGenerate_R_Symbols(t *testing.T) {
	src := `
setClass("MyClass", representation(x = "integer"))
my_func <- function(x) x
`
	fm, ok := Generate(src, "sym.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	var hasClass, hasFn bool
	for _, sym := range fm.Symbols {
		if sym == "class MyClass" {
			hasClass = true
		}
		if sym == "fn my_func" {
			hasFn = true
		}
	}
	if !hasClass {
		t.Errorf("Symbols %v missing 'class MyClass'", fm.Symbols)
	}
	if !hasFn {
		t.Errorf("Symbols %v missing 'fn my_func'", fm.Symbols)
	}
}

// TestGenerate_R_FallsThrough_Empty: empty file falls through.
func TestGenerate_R_FallsThrough_Empty(t *testing.T) {
	_, ok := Generate("", "empty.R")
	if ok {
		t.Fatal("expected ok=false for empty file")
	}
}

// TestGenerate_R_FallsThrough_NoFunctions: file with no function definitions
// falls through (pure library/option calls).
func TestGenerate_R_FallsThrough_NoFunctions(t *testing.T) {
	src := `
library(ggplot2)
library(dplyr)
options(stringsAsFactors = FALSE)
`
	_, ok := Generate(src, "setup.R")
	if ok {
		t.Fatal("expected ok=false for file with no function definitions")
	}
}

// TestGenerate_R_Deduplication: same function name assigned twice appears once.
func TestGenerate_R_Deduplication(t *testing.T) {
	src := `
foo <- function(x) x + 1
foo <- function(x) x + 2  # redefine
`
	fm, ok := Generate(src, "dup.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Count(kind, "foo") != 1 {
		t.Errorf("expected 'foo' once in kind %q", kind)
	}
}

// TestGenerate_R_InlineFunction: single-line function with no braces.
func TestGenerate_R_InlineFunction(t *testing.T) {
	src := `
square <- function(x) x^2

cube <- function(x) x^3
`
	fm, ok := Generate(src, "math.R")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "square") {
		t.Errorf("kind %q missing 'square'", kind)
	}
	if !strings.Contains(kind, "cube") {
		t.Errorf("kind %q missing 'cube'", kind)
	}
}
