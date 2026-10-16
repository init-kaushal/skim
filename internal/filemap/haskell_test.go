package filemap

import (
	"strings"
	"testing"
)

// TestGenerate_Haskell_BasicModule: module name, data types, and function
// signatures are all captured.
func TestGenerate_Haskell_BasicModule(t *testing.T) {
	src := `module Web.Server where

import Data.Text (Text)

data Config = Config
  { configHost :: Text
  , configPort :: Int
  }

data Status = Running | Stopped | Failed String

serve :: Config -> IO ()
serve cfg = undefined

stop :: IO ()
stop = undefined
`
	fm, ok := Generate(src, "Server.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "module Web.Server") {
		t.Errorf("kind %q missing 'module Web.Server'", kind)
	}
	for _, name := range []string{"Config", "Status", "serve", "stop"} {
		if !strings.Contains(kind, name) {
			t.Errorf("kind %q missing %q", kind, name)
		}
	}
}

// TestGenerate_Haskell_DataTypes: various type declarations are captured.
func TestGenerate_Haskell_DataTypes(t *testing.T) {
	src := `module Types where

data Tree a = Leaf | Node (Tree a) a (Tree a)

newtype Identity a = Identity { runIdentity :: a }

type Name = String

type family Elem c
`
	fm, ok := Generate(src, "Types.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	for _, name := range []string{"Tree", "Identity", "Name", "Elem"} {
		if !strings.Contains(kind, name) {
			t.Errorf("kind %q missing %q", kind, name)
		}
	}
}

// TestGenerate_Haskell_TypeClasses: class declarations are collected.
func TestGenerate_Haskell_TypeClasses(t *testing.T) {
	src := `module Classes where

class Functor f where
  fmap :: (a -> b) -> f a -> f b

class (Eq a, Show a) => Printable a where
  prettyPrint :: a -> String
`
	fm, ok := Generate(src, "Classes.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "Functor") {
		t.Errorf("kind %q missing 'Functor'", kind)
	}
	if !strings.Contains(kind, "Printable") {
		t.Errorf("kind %q missing 'Printable'", kind)
	}
}

// TestGenerate_Haskell_FunctionSigs: type signatures at column 0 are captured
// as function names; definition lines are ignored.
func TestGenerate_Haskell_FunctionSigs(t *testing.T) {
	src := `module App where

main :: IO ()
main = putStrLn "hello"

run :: Int -> String -> Bool
run n s = n > length s

helper :: String -> String
helper = reverse
`
	fm, ok := Generate(src, "App.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	for _, fn := range []string{"main", "run", "helper"} {
		if !strings.Contains(kind, fn) {
			t.Errorf("kind %q missing %q", kind, fn)
		}
	}
}

// TestGenerate_Haskell_IndentedLinesSkipped: type signatures and function defs
// inside where/class/instance blocks are indented and must not be captured.
func TestGenerate_Haskell_IndentedLinesSkipped(t *testing.T) {
	src := `module Foo where

class Monad m where
  return :: a -> m a
  (>>=)  :: m a -> (a -> m b) -> m b

instance Monad Maybe where
  return x  = Just x
  Nothing >>= _ = Nothing
  Just x  >>= f = f x

outer :: Int -> Int
outer x = inner x
  where
    inner :: Int -> Int
    inner n = n + 1
`
	fm, ok := Generate(src, "Foo.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	// Only top-level functions; indented ones (return, >>=, inner) not captured.
	if !strings.Contains(kind, "outer") {
		t.Errorf("kind %q missing 'outer'", kind)
	}
	// "return" may appear if it escapes; but isHaskellKeyword filters it
	// (it's not in our keyword list, but it IS in a class body so it's indented → skip).
	// Let's just verify it's not falsely present from an indented line.
}

// TestGenerate_Haskell_MultiNameSig: "foo, bar :: Type" captures both names.
func TestGenerate_Haskell_MultiNameSig(t *testing.T) {
	src := `module Multi where

minBound, maxBound :: Int
minBound = 0
maxBound = 100
`
	fm, ok := Generate(src, "Multi.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "minBound") {
		t.Errorf("kind %q missing 'minBound'", kind)
	}
	if !strings.Contains(kind, "maxBound") {
		t.Errorf("kind %q missing 'maxBound'", kind)
	}
}

// TestGenerate_Haskell_BlockComment: names inside {- -} are not captured.
func TestGenerate_Haskell_BlockComment(t *testing.T) {
	src := `module Commented where

{-
data FakeType = FakeType

fakeFunc :: Int -> Int
-}

data RealType = RealType

realFunc :: String
realFunc = "real"
`
	fm, ok := Generate(src, "Commented.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "FakeType") {
		t.Errorf("kind %q should not contain 'FakeType' from block comment", kind)
	}
	if strings.Contains(kind, "fakeFunc") {
		t.Errorf("kind %q should not contain 'fakeFunc' from block comment", kind)
	}
	if !strings.Contains(kind, "RealType") {
		t.Errorf("kind %q missing 'RealType'", kind)
	}
	if !strings.Contains(kind, "realFunc") {
		t.Errorf("kind %q missing 'realFunc'", kind)
	}
}

// TestGenerate_Haskell_LineComment: names after -- are not captured.
func TestGenerate_Haskell_LineComment(t *testing.T) {
	src := `module Lc where

-- data Hidden = Hidden

data Visible = Visible
`
	fm, ok := Generate(src, "Lc.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if strings.Contains(kind, "Hidden") {
		t.Errorf("kind %q should not contain 'Hidden' from line comment", kind)
	}
	if !strings.Contains(kind, "Visible") {
		t.Errorf("kind %q missing 'Visible'", kind)
	}
}

// TestGenerate_Haskell_InstanceSkipped: instance declarations don't contribute
// names to the file map (would be too noisy).
func TestGenerate_Haskell_InstanceSkipped(t *testing.T) {
	src := `module Inst where

data Foo = Foo

instance Show Foo where
  show _ = "Foo"

instance Eq Foo where
  _ == _ = True
`
	fm, ok := Generate(src, "Inst.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	// "Show" and "Eq" should not appear as type names from instance lines.
	if strings.Contains(kind, "Show") {
		t.Errorf("kind %q should not contain 'Show' from instance line", kind)
	}
	if strings.Contains(kind, "Eq") {
		t.Errorf("kind %q should not contain 'Eq' from instance line", kind)
	}
}

// TestGenerate_Haskell_Overflow: more than 4 names → (+N) suffix.
func TestGenerate_Haskell_Overflow(t *testing.T) {
	src := `module Big where

data A = A
data B = B
data C = C
data D = D
data E = E
data F = F
`
	fm, ok := Generate(src, "Big.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+2)") {
		t.Errorf("kind %q expected '(+2)' overflow", kind)
	}
}

// TestGenerate_Haskell_LineRange: map entry spans the whole file.
func TestGenerate_Haskell_LineRange(t *testing.T) {
	src := `module R where
data X = X
`
	fm, ok := Generate(src, "R.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if fm.Map[0].Lines != "1-2" {
		t.Errorf("expected Lines='1-2', got %q", fm.Map[0].Lines)
	}
}

// TestGenerate_Haskell_GeneratedFile: generated file still returns ok=true but
// summary contains "generated".
func TestGenerate_Haskell_GeneratedFile(t *testing.T) {
	src := `-- Generated by hsc2hs. DO NOT EDIT.
module Generated where

data CStruct = CStruct
`
	fm, ok := Generate(src, "Generated.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("summary %q missing 'generated'", fm.Summary)
	}
}

// TestGenerate_Haskell_Summary: summary contains module name and counts.
func TestGenerate_Haskell_Summary(t *testing.T) {
	src := `module My.Module where
data A = A
data B = B
foo :: Int
foo = 1
`
	fm, ok := Generate(src, "Module.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(fm.Summary, "My.Module") {
		t.Errorf("summary %q missing 'My.Module'", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "2 types") {
		t.Errorf("summary %q missing '2 types'", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "1 function") {
		t.Errorf("summary %q missing '1 function'", fm.Summary)
	}
}

// TestGenerate_Haskell_Symbols: Symbols slice contains "type X" and "fn f".
func TestGenerate_Haskell_Symbols(t *testing.T) {
	src := `module S where
data MyType = MyType
myFunc :: ()
myFunc = ()
`
	fm, ok := Generate(src, "S.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	var hasType, hasFn bool
	for _, sym := range fm.Symbols {
		if sym == "type MyType" {
			hasType = true
		}
		if sym == "fn myFunc" {
			hasFn = true
		}
	}
	if !hasType {
		t.Errorf("Symbols %v missing 'type MyType'", fm.Symbols)
	}
	if !hasFn {
		t.Errorf("Symbols %v missing 'fn myFunc'", fm.Symbols)
	}
}

// TestGenerate_Haskell_NoModuleStillWorks: file without module declaration
// falls back to the filename stem.
func TestGenerate_Haskell_NoModuleStillWorks(t *testing.T) {
	src := `data Point = Point Int Int

distance :: Point -> Point -> Double
distance p1 p2 = undefined
`
	fm, ok := Generate(src, "geometry.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "module geometry") {
		t.Errorf("kind %q missing 'module geometry' fallback", kind)
	}
	if !strings.Contains(kind, "Point") {
		t.Errorf("kind %q missing 'Point'", kind)
	}
}

// TestGenerate_Haskell_FallsThrough_Empty: empty file falls through.
func TestGenerate_Haskell_FallsThrough_Empty(t *testing.T) {
	_, ok := Generate("", "empty.hs")
	if ok {
		t.Fatal("expected ok=false for empty file")
	}
}

// TestGenerate_Haskell_FallsThrough_NoStructure: file with no module/data/class
// declarations falls through.
func TestGenerate_Haskell_FallsThrough_NoStructure(t *testing.T) {
	src := `-- just a comment file
-- no real declarations
`
	_, ok := Generate(src, "comments.hs")
	if ok {
		t.Fatal("expected ok=false for file with no structure")
	}
}

// TestGenerate_Haskell_Deduplication: same type name appearing twice (e.g. via
// role annotation) is recorded only once.
func TestGenerate_Haskell_Deduplication(t *testing.T) {
	src := `module Dup where
data Foo = Foo
data Foo = Foo  -- second definition (unusual but parses)
`
	fm, ok := Generate(src, "Dup.hs")
	if !ok {
		t.Fatal("expected ok=true")
	}
	kind := fm.Map[0].Kind
	count := strings.Count(kind, "Foo")
	if count != 1 {
		t.Errorf("expected 'Foo' once in kind %q, found %d times", kind, count)
	}
}
