package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_GraphQL_TypesAndInputs(t *testing.T) {
	src := `type User {
  id:        ID!
  email:     String!
  name:      String
  createdAt: String!
}

type Post {
  id:      ID!
  title:   String!
  body:    String
  author:  User!
}

input CreateUserInput {
  email: String!
  name:  String!
}

input UpdatePostInput {
  title: String
  body:  String
}
`
	fm, ok := filemap.Generate(src, "schema.graphql")
	if !ok {
		t.Fatal("expected deterministic map for GraphQL schema")
	}

	if !strings.Contains(fm.Summary, "GraphQL schema") {
		t.Errorf("expected 'GraphQL schema' prefix, got: %s", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "type") {
		t.Errorf("expected 'type' count in summary, got: %s", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "input") {
		t.Errorf("expected 'input' count in summary, got: %s", fm.Summary)
	}

	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	for _, want := range []string{"type User", "type Post", "input CreateUserInput", "input UpdatePostInput"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q in %v", want, fm.Symbols)
		}
	}
}

func TestGenerate_GraphQL_EnumAndInterface(t *testing.T) {
	src := `interface Node {
  id: ID!
}

interface Timestamped {
  createdAt: String!
  updatedAt: String!
}

enum UserRole {
  ADMIN
  EDITOR
  VIEWER
}

enum PostStatus {
  DRAFT
  PUBLISHED
  ARCHIVED
}
`
	fm, ok := filemap.Generate(src, "types.graphql")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	if !symbolSet["interface Node"] {
		t.Errorf("expected 'interface Node' in %v", fm.Symbols)
	}
	if !symbolSet["enum UserRole"] {
		t.Errorf("expected 'enum UserRole' in %v", fm.Symbols)
	}
	if len(fm.Map) != 4 {
		t.Errorf("expected 4 entries, got %d", len(fm.Map))
	}
}

func TestGenerate_GraphQL_UnionAndScalar(t *testing.T) {
	// Unions and scalars have no body braces — they're single-line definitions.
	src := `scalar DateTime
scalar UUID
scalar JSON

union SearchResult = User | Post | Comment

union Notification = LikeNotification | CommentNotification
`
	fm, ok := filemap.Generate(src, "scalars.graphql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 5 {
		t.Fatalf("expected 5 entries (3 scalars + 2 unions), got %d: %v", len(fm.Map), fm.Map)
	}
}

func TestGenerate_GraphQL_Operations(t *testing.T) {
	src := `query GetUser($id: ID!) {
  user(id: $id) {
    id
    email
    name
  }
}

query ListUsers($limit: Int, $offset: Int) {
  users(limit: $limit, offset: $offset) {
    id
    email
  }
}

mutation CreateUser($input: CreateUserInput!) {
  createUser(input: $input) {
    id
    email
  }
}
`
	fm, ok := filemap.Generate(src, "user_ops.graphql")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	if !strings.Contains(fm.Summary, "GraphQL operations") {
		t.Errorf("expected 'GraphQL operations' prefix, got: %s", fm.Summary)
	}
	if len(fm.Map) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	if !symbolSet["query GetUser"] {
		t.Errorf("expected 'query GetUser' in %v", fm.Symbols)
	}
	if !symbolSet["mutation CreateUser"] {
		t.Errorf("expected 'mutation CreateUser' in %v", fm.Symbols)
	}
}

func TestGenerate_GraphQL_Fragment(t *testing.T) {
	src := `fragment UserFields on User {
  id
  email
  name
}

fragment PostFields on Post {
  id
  title
  author {
    ...UserFields
  }
}
`
	fm, ok := filemap.Generate(src, "fragments.graphql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 fragment entries, got %d: %v", len(fm.Map), fm.Map)
	}
}

func TestGenerate_GraphQL_DescriptionStrings(t *testing.T) {
	// Triple-quoted block strings (descriptions) must not confuse brace counting.
	src := `"""
Schema for the user management API.
Supports CRUD operations on users.
"""

type User {
  """The user's unique identifier."""
  id: ID!
  """
  The user's email address.
  Must be unique across all users.
  """
  email: String!
}

type Query {
  """Fetch a user by ID."""
  user(id: ID!): User
}
`
	fm, ok := filemap.Generate(src, "described.graphql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (block string skipping broken?), got %d: %v", len(fm.Map), fm.Map)
	}
}

func TestGenerate_GraphQL_BraceInString(t *testing.T) {
	// A closing brace inside a string description must not close the block early.
	src := `type Config {
  pattern: String!
  description: String!
}

type Rule {
  name: String!
}
`
	fm, ok := filemap.Generate(src, "config.graphql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(fm.Map), fm.Map)
	}
}

func TestGenerate_GraphQL_ExtendType(t *testing.T) {
	src := `extend type User {
  role: UserRole!
  posts: [Post!]!
}

extend input CreateUserInput {
  role: UserRole
}
`
	fm, ok := filemap.Generate(src, "extend.graphql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 extend entries, got %d: %v", len(fm.Map), fm.Map)
	}
}

// TestGenerate_GraphQL_MultilineUnion is the regression test for the bug where
// a union formatted across multiple lines got a line range pointing only to
// the declaration line, omitting the pipe-member lines.
func TestGenerate_GraphQL_MultilineUnion(t *testing.T) {
	src := `type Photo {
  id: ID!
  url: String!
}

type Person {
  id: ID!
  name: String!
}

type Article {
  id: ID!
  title: String!
}

union SearchResult =
  | Photo
  | Person
  | Article

type Query {
  search(term: String!): [SearchResult!]!
}
`
	fm, ok := filemap.Generate(src, "search.graphql")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	// Find the SearchResult union entry.
	var unionEntry *struct{ Lines, Kind string }
	for _, e := range fm.Map {
		if strings.Contains(e.Kind, "SearchResult") {
			unionEntry = &struct{ Lines, Kind string }{e.Lines, e.Kind}
		}
	}
	if unionEntry == nil {
		t.Fatal("expected union SearchResult entry in map")
	}

	// The union starts at line 16 ("union SearchResult =") and ends at line 19
	// ("  | Article"). Verify both bounds.
	if !strings.HasPrefix(unionEntry.Lines, "16") {
		t.Errorf("union should start at line 16, got: %s", unionEntry.Lines)
	}
	if !strings.HasSuffix(unionEntry.Lines, "19") {
		t.Errorf("union should end at line 19 (last | member), got: %s", unionEntry.Lines)
	}

	// Inline union (all members on one line) should still get a single-line range.
	src2 := `union InlineUnion = Foo | Bar | Baz

type Foo { id: ID! }
type Bar { id: ID! }
type Baz { id: ID! }
`
	fm2, ok2 := filemap.Generate(src2, "inline.graphql")
	if !ok2 {
		t.Fatal("expected deterministic map for inline union")
	}
	for _, e := range fm2.Map {
		if e.Kind == "union InlineUnion" {
			// Inline union: start == end (single line).
			parts := strings.SplitN(e.Lines, "-", 2)
			if len(parts) == 2 && parts[0] != parts[1] {
				t.Errorf("inline union should have single-line range, got: %s", e.Lines)
			}
			return
		}
	}
	t.Error("expected union InlineUnion entry in map")
}

func TestGenerate_GraphQL_GeneratedFile(t *testing.T) {
	src := `# This file was automatically generated by graphql-codegen. DO NOT EDIT.

type User {
  id: ID!
  email: String!
}
`
	fm, ok := filemap.Generate(src, "generated.graphql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_GraphQL_LineRanges(t *testing.T) {
	src := `type Alpha {
  id: ID!
}

type Beta {
  name: String!
}

type Gamma {
  value: Int!
}
`
	fm, ok := filemap.Generate(src, "ranges.graphql")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(fm.Map))
	}
	if !strings.HasPrefix(fm.Map[0].Lines, "1") {
		t.Errorf("Alpha should start at line 1, got: %s", fm.Map[0].Lines)
	}
	if !strings.HasPrefix(fm.Map[1].Lines, "5") {
		t.Errorf("Beta should start at line 5, got: %s", fm.Map[1].Lines)
	}
}

func TestGenerate_GraphQL_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "schema.graphql")
	if ok {
		t.Error("expected fallthrough for empty GraphQL file")
	}
}

func TestGenerate_GraphQL_FallsThrough_NoDefinitions(t *testing.T) {
	// A file with only comments and directives but no type definitions.
	src := `# comment only file
# no types here
`
	_, ok := filemap.Generate(src, "empty.graphql")
	if ok {
		t.Error("expected fallthrough for GraphQL file with no definitions")
	}
}

func TestGenerate_GraphQL_DotGQL(t *testing.T) {
	// .gql extension should also be recognized.
	src := `type Product {
  id: ID!
  name: String!
  price: Float!
}
`
	fm, ok := filemap.Generate(src, "products.gql")
	if !ok {
		t.Fatal("expected deterministic map for .gql extension")
	}
	if len(fm.Map) != 1 {
		t.Errorf("expected 1 entry, got %d", len(fm.Map))
	}
}
