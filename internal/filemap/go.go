package filemap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"unicode"

	"github.com/kaushal/skim/internal/digest"
)

// parseGoFile builds a FileMap by walking the Go AST. It uses partial parse
// results when there are syntax errors so that files with minor issues still
// get a useful map. Returns an error only when no AST was produced at all.
func parseGoFile(src, filePath string) (digest.FileMap, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	// A nil file or missing package declaration means the content is not valid
	// Go. Fall through to the worker path.
	if f == nil || f.Name == nil || f.Name.Name == "" {
		if err != nil {
			return digest.FileMap{}, err
		}
		return digest.FileMap{}, fmt.Errorf("filemap: no package declaration")
	}

	var (
		entries []digest.MapEntry
		symbols []string
		notes   []string
	)

	// Summary: prefer the package-level doc comment; fall back to listing the
	// package name and whether this is a generated file.
	summary := packageSummary(f, filePath)

	if isGenerated(f) {
		notes = append(notes, "generated file — edit the generator, not this file")
	}

	// Walk top-level declarations to build the structural map.
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			entry, syms := genDeclMapEntry(fset, d)
			if entry.Lines != "" {
				entries = append(entries, entry)
			}
			symbols = append(symbols, syms...)

		case *ast.FuncDecl:
			entry, sym := funcDeclMapEntry(fset, d)
			if entry.Lines != "" {
				entries = append(entries, entry)
			}
			if sym != "" {
				symbols = append(symbols, sym)
			}
		}
	}

	// Deduplicate symbols while preserving order.
	symbols = dedupe(symbols)

	fm := digest.FileMap{
		Summary: summary,
		Map:     entries,
		Symbols: symbols,
	}
	if len(notes) > 0 {
		fm.Notes = strings.Join(notes, "; ")
	}

	// Ensure Map is non-empty (required by ParseFileMap). If the file has no
	// top-level declarations (blank file, comment-only file), add one entry
	// covering the whole file.
	if len(fm.Map) == 0 {
		fm.Map = []digest.MapEntry{{
			Lines: fmt.Sprintf("1-%d", lineCount(src)),
			Kind:  fmt.Sprintf("package %s (no exported declarations)", f.Name.Name),
		}}
	}

	return fm, nil
}

// ── Summary generation ────────────────────────────────────────────────────────

func packageSummary(f *ast.File, filePath string) string {
	pkg := f.Name.Name
	if f.Doc != nil && f.Doc.Text() != "" {
		doc := firstSentence(strings.TrimSpace(f.Doc.Text()))
		if doc != "" {
			// "Package foo provides..." → use as-is
			// Otherwise prefix with "Package foo —"
			lower := strings.ToLower(doc)
			if strings.HasPrefix(lower, "package ") {
				return doc
			}
			return fmt.Sprintf("Package %s — %s", pkg, doc)
		}
	}
	return fmt.Sprintf("Package %s", pkg)
}

func isGenerated(f *ast.File) bool {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, "Code generated") ||
				strings.Contains(c.Text, "DO NOT EDIT") {
				return true
			}
		}
	}
	return false
}

// ── GenDecl (import / const / var / type) ─────────────────────────────────────

func genDeclMapEntry(fset *token.FileSet, d *ast.GenDecl) (digest.MapEntry, []string) {
	start := fset.Position(d.Pos()).Line
	end := fset.Position(d.End()).Line
	lineStr := lineRange(start, end)

	var syms []string

	switch d.Tok.String() {
	case "import":
		n := len(d.Specs)
		return digest.MapEntry{Lines: lineStr, Kind: fmt.Sprintf("imports (%d packages)", n)}, nil

	case "type":
		if len(d.Specs) == 1 {
			ts := d.Specs[0].(*ast.TypeSpec)
			kind := typeKind(ts)
			label := fmt.Sprintf("type %s %s", ts.Name.Name, kind)
			if doc := leadingDoc(d, ts); doc != "" {
				label += " — " + doc
			}
			if ts.Name.IsExported() {
				syms = append(syms, ts.Name.Name)
			}
			return digest.MapEntry{Lines: lineStr, Kind: label}, syms
		}
		// Multiple types in one block (rare)
		var names []string
		for _, spec := range d.Specs {
			ts := spec.(*ast.TypeSpec)
			if ts.Name.IsExported() {
				names = append(names, ts.Name.Name)
				syms = append(syms, ts.Name.Name)
			}
		}
		if len(names) == 0 {
			names = specNames(d.Specs)
		}
		return digest.MapEntry{Lines: lineStr, Kind: fmt.Sprintf("type %s", strings.Join(names, ", "))}, syms

	case "const":
		names := exportedNames(d.Specs)
		if len(names) == 0 {
			return digest.MapEntry{}, nil // all unexported, skip
		}
		syms = append(syms, names...)
		label := fmt.Sprintf("const %s", joinMax(names, 3))
		if doc := d.Doc.Text(); doc != "" {
			label += " — " + firstSentence(doc)
		}
		return digest.MapEntry{Lines: lineStr, Kind: label}, syms

	case "var":
		names := exportedNames(d.Specs)
		if len(names) == 0 {
			return digest.MapEntry{}, nil
		}
		syms = append(syms, names...)
		label := fmt.Sprintf("var %s", joinMax(names, 3))
		if doc := d.Doc.Text(); doc != "" {
			label += " — " + firstSentence(doc)
		}
		return digest.MapEntry{Lines: lineStr, Kind: label}, syms
	}

	return digest.MapEntry{}, nil
}

func typeKind(ts *ast.TypeSpec) string {
	switch t := ts.Type.(type) {
	case *ast.StructType:
		n := 0
		for _, f := range t.Fields.List {
			n += max(len(f.Names), 1)
		}
		if n == 1 {
			return "struct (1 field)"
		}
		return fmt.Sprintf("struct (%d fields)", n)
	case *ast.InterfaceType:
		n := len(t.Methods.List)
		if n == 1 {
			return "interface (1 method)"
		}
		return fmt.Sprintf("interface (%d methods)", n)
	case *ast.MapType:
		return "map type"
	case *ast.ArrayType:
		if t.Len == nil {
			return "slice type"
		}
		return "array type"
	case *ast.FuncType:
		return "func type"
	case *ast.Ident:
		return fmt.Sprintf("= %s", t.Name)
	default:
		return "type"
	}
}

// leadingDoc extracts the most relevant doc comment for a single-spec GenDecl:
// the spec's own doc comment if present, otherwise the GenDecl's doc comment.
func leadingDoc(d *ast.GenDecl, spec ast.Spec) string {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		if s.Comment != nil && s.Comment.Text() != "" {
			return firstSentence(s.Comment.Text())
		}
		if s.Doc != nil && s.Doc.Text() != "" {
			return firstSentence(s.Doc.Text())
		}
	}
	if d.Doc != nil && d.Doc.Text() != "" {
		return firstSentence(d.Doc.Text())
	}
	return ""
}

// ── FuncDecl ─────────────────────────────────────────────────────────────────

func funcDeclMapEntry(fset *token.FileSet, d *ast.FuncDecl) (digest.MapEntry, string) {
	start := fset.Position(d.Pos()).Line
	end := fset.Position(d.End()).Line

	sig := funcSignature(d)
	label := sig
	if doc := funcDoc(d); doc != "" {
		label += " — " + doc
	}

	sym := ""
	if d.Name.IsExported() {
		sym = sig
	}

	return digest.MapEntry{Lines: lineRange(start, end), Kind: label}, sym
}

func funcSignature(d *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("func ")
	if d.Recv != nil && len(d.Recv.List) > 0 {
		b.WriteString("(")
		b.WriteString(receiverType(d.Recv.List[0]))
		b.WriteString(") ")
	}
	b.WriteString(d.Name.Name)
	b.WriteString("(")
	b.WriteString(paramList(d.Type.Params))
	b.WriteString(")")
	if ret := returnList(d.Type.Results); ret != "" {
		b.WriteString(" ")
		b.WriteString(ret)
	}
	return b.String()
}

func funcDoc(d *ast.FuncDecl) string {
	if d.Doc == nil {
		return ""
	}
	return firstSentence(strings.TrimSpace(d.Doc.Text()))
}

func receiverType(f *ast.Field) string {
	switch t := f.Type.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return "*" + id.Name
		}
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name + "[T]"
		}
	}
	return "T"
}

func paramList(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	const maxParams = 3
	var parts []string
	for _, f := range fl.List {
		typStr := typeString(f.Type)
		if len(f.Names) == 0 {
			parts = append(parts, typStr)
		} else {
			for _, n := range f.Names {
				parts = append(parts, n.Name+" "+typStr)
			}
		}
	}
	if len(parts) > maxParams {
		return strings.Join(parts[:maxParams], ", ") + ", ..."
	}
	return strings.Join(parts, ", ")
}

func returnList(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	if len(fl.List) == 1 && len(fl.List[0].Names) == 0 {
		return typeString(fl.List[0].Type)
	}
	var parts []string
	for _, f := range fl.List {
		parts = append(parts, typeString(f.Type))
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// typeString produces a compact representation of an AST type expression.
func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + typeString(t.Elt)
		}
		return "[...]" + typeString(t.Elt)
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + typeString(t.Value)
	case *ast.Ellipsis:
		return "..." + typeString(t.Elt)
	case *ast.IndexExpr:
		return typeString(t.X) + "[" + typeString(t.Index) + "]"
	}
	return "..."
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func lineRange(start, end int) string {
	if start == end {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Find first sentence boundary (. ! ?) followed by space or end.
	for i, r := range s {
		if (r == '.' || r == '!' || r == '?') && (i+1 >= len(s) || unicode.IsSpace(rune(s[i+1]))) {
			return strings.TrimSpace(s[:i+1])
		}
	}
	// No sentence boundary — cap at 80 chars.
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}

func exportedNames(specs []ast.Spec) []string {
	var names []string
	for _, spec := range specs {
		switch s := spec.(type) {
		case *ast.ValueSpec:
			for _, n := range s.Names {
				if n.IsExported() {
					names = append(names, n.Name)
				}
			}
		case *ast.TypeSpec:
			if s.Name.IsExported() {
				names = append(names, s.Name.Name)
			}
		}
	}
	return names
}

func specNames(specs []ast.Spec) []string {
	var names []string
	for _, spec := range specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			names = append(names, s.Name.Name)
		case *ast.ValueSpec:
			for _, n := range s.Names {
				names = append(names, n.Name)
			}
		}
	}
	return names
}

func joinMax(ss []string, n int) string {
	if len(ss) <= n {
		return strings.Join(ss, ", ")
	}
	return strings.Join(ss[:n], ", ") + fmt.Sprintf(" (+%d more)", len(ss)-n)
}

func dedupe(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := ss[:0:0]
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func lineCount(src string) int {
	n := strings.Count(src, "\n")
	if n == 0 {
		return 1
	}
	return n
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
