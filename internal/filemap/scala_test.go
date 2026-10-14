package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_Scala_ClassWithDefs(t *testing.T) {
	src := `package com.example.service

import scala.concurrent.Future
import scala.concurrent.ExecutionContext

class UserService(repo: UserRepository)(implicit ec: ExecutionContext) {

  def createUser(email: String, name: String): Future[User] = {
    val user = User(email = email, name = name)
    repo.save(user)
  }

  def findUser(id: Long): Future[Option[User]] =
    repo.findById(id)

  def deleteUser(id: Long): Future[Unit] =
    repo.deleteById(id)
}
`
	fm, ok := filemap.Generate(src, "UserService.scala")
	if !ok {
		t.Fatal("expected deterministic map for Scala file")
	}
	if !strings.Contains(fm.Summary, "Scala") {
		t.Errorf("expected 'Scala' in summary, got: %s", fm.Summary)
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 class entry, got %d: %v", len(fm.Map), fm.Map)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "class UserService") {
		t.Errorf("expected 'class UserService' in Kind, got: %s", kind)
	}
	for _, d := range []string{"createUser", "findUser", "deleteUser"} {
		if !strings.Contains(kind, d) {
			t.Errorf("expected def %q in Kind, got: %s", d, kind)
		}
	}
}

func TestGenerate_Scala_CaseClass(t *testing.T) {
	src := `package com.example.model

case class User(
  id: Long,
  email: String,
  name: String,
  active: Boolean = true
) {
  def displayName: String = s"$name <$email>"
  def isAdmin: Boolean = false
}
`
	fm, ok := filemap.Generate(src, "User.scala")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "case class User") {
		t.Errorf("expected 'case class User' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[0].Kind, "displayName") {
		t.Errorf("expected 'displayName' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "case class") {
		t.Errorf("expected 'case class' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Scala_Trait(t *testing.T) {
	src := `package com.example.repository

import scala.concurrent.Future

trait UserRepository {
  def findById(id: Long): Future[Option[User]]
  def findAll(): Future[Seq[User]]
  def save(user: User): Future[User]
  def deleteById(id: Long): Future[Unit]
}
`
	fm, ok := filemap.Generate(src, "UserRepository.scala")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "trait UserRepository") {
		t.Errorf("expected 'trait UserRepository' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "trait") {
		t.Errorf("expected 'trait' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Scala_Object(t *testing.T) {
	src := `package com.example.util

object StringUtils {
  def capitalize(s: String): String =
    if (s.isEmpty) s else s.head.toUpper +: s.tail

  def truncate(s: String, maxLen: Int): String =
    if (s.length <= maxLen) s else s.take(maxLen) + "..."

  def slugify(s: String): String =
    s.toLowerCase.replaceAll("[^a-z0-9]+", "-")
}
`
	fm, ok := filemap.Generate(src, "StringUtils.scala")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "object StringUtils") {
		t.Errorf("expected 'object StringUtils' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "object") {
		t.Errorf("expected 'object' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Scala_CompanionObject(t *testing.T) {
	src := `package com.example.model

class Account(val id: Long, val balance: BigDecimal) {
  def deposit(amount: BigDecimal): Account = copy(balance = balance + amount)
  def withdraw(amount: BigDecimal): Account = copy(balance = balance - amount)
  private def copy(balance: BigDecimal): Account = new Account(id, balance)
}

object Account {
  def apply(id: Long): Account = new Account(id, BigDecimal(0))
  def empty: Account = new Account(0L, BigDecimal(0))
}
`
	fm, ok := filemap.Generate(src, "Account.scala")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (class + companion object), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "class Account") {
		t.Errorf("expected 'class Account' as first entry, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "object Account") {
		t.Errorf("expected 'object Account' as second entry, got: %s", fm.Map[1].Kind)
	}
	if !strings.Contains(fm.Summary, "class") || !strings.Contains(fm.Summary, "object") {
		t.Errorf("expected both 'class' and 'object' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Scala_SealedTrait(t *testing.T) {
	src := `package com.example.model

sealed trait UserEvent {
  def userId: Long
}

case class UserCreated(userId: Long, email: String) extends UserEvent
case class UserDeleted(userId: Long) extends UserEvent
case class UserUpdated(userId: Long, changes: Map[String, String]) extends UserEvent
`
	fm, ok := filemap.Generate(src, "UserEvent.scala")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 4 {
		t.Fatalf("expected 4 entries (sealed trait + 3 case classes), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "sealed trait UserEvent") {
		t.Errorf("expected 'sealed trait UserEvent' as first entry, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "sealed trait") || !strings.Contains(fm.Summary, "case class") {
		t.Errorf("expected 'sealed trait' and 'case class' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Scala_DefOverflow(t *testing.T) {
	src := `class ApiController(service: ApiService) {
  def index(): Result = service.findAll()
  def show(id: Long): Result = service.findById(id)
  def create(dto: CreateDto): Result = service.create(dto)
  def update(id: Long, dto: UpdateDto): Result = service.update(id, dto)
  def delete(id: Long): Result = service.delete(id)
}
`
	fm, ok := filemap.Generate(src, "ApiController.scala")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+1)") {
		t.Errorf("expected (+1) for 5th def, got: %s", kind)
	}
}

// TestGenerate_Scala_BlockComment verifies that braces inside /* ... */ block
// comments do not corrupt depth tracking.
func TestGenerate_Scala_BlockComment(t *testing.T) {
	src := `class Example {
  /* } */ def run(): Unit = {}
}

class Another {
  def check(): Boolean = true
}
`
	fm, ok := filemap.Generate(src, "example.scala")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (block comment '}' corrupted depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[1].Kind, "Another") {
		t.Errorf("expected Another as second entry, got: %s", fm.Map[1].Kind)
	}
}

// TestGenerate_Scala_TripleQuote verifies that braces inside triple-quoted
// strings do not corrupt depth tracking.
func TestGenerate_Scala_TripleQuote(t *testing.T) {
	src := `class SqlQueries {
  val createTable: String = """
    CREATE TABLE users (
      id BIGSERIAL PRIMARY KEY,
      email TEXT NOT NULL
    )
  """

  def run(db: Database): Unit = db.execute(createTable)
}

class AfterTriple {
  def check(): Unit = ()
}
`
	fm, ok := filemap.Generate(src, "queries.scala")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (triple-quote broke depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "run") {
		t.Errorf("expected 'run' in SqlQueries Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "AfterTriple") {
		t.Errorf("expected AfterTriple as second entry, got: %s", fm.Map[1].Kind)
	}
}

func TestGenerate_Scala_LineRanges(t *testing.T) {
	src := `class Alpha {
  def hello(): Unit = ()
}

class Beta {
  def world(): Unit = ()
}
`
	fm, ok := filemap.Generate(src, "ranges.scala")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(fm.Map))
	}
	if !strings.HasPrefix(fm.Map[0].Lines, "1") {
		t.Errorf("Alpha should start at line 1, got: %s", fm.Map[0].Lines)
	}
	if !strings.HasPrefix(fm.Map[1].Lines, "5") {
		t.Errorf("Beta should start at line 5, got: %s", fm.Map[1].Lines)
	}
}

func TestGenerate_Scala_GeneratedFile(t *testing.T) {
	src := `// Generated by ScalaPB. DO NOT EDIT.
package com.example.proto

case class UserProto(id: Long, email: String) {
  def toByteArray: Array[Byte] = ???
}
`
	fm, ok := filemap.Generate(src, "UserProto.scala")
	if !ok {
		t.Fatal("expected deterministic map for generated Scala file")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in summary, got: %s", fm.Summary)
	}
}

// TestGenerate_Scala_NestedBlockComment is the regression test for nested
// Scala block comments (/* /* ... */ */). The previous *bcDepth = 1 approach
// reset depth to 1 on every "/*", so an inner "*/" would exit the comment
// while still inside the outer one. Subsequent braces (including the "}" in
// the outer comment body) then corrupted depth tracking.
func TestGenerate_Scala_NestedBlockComment(t *testing.T) {
	src := `class Example {
  /*
    outer comment
    /* nested comment */ } // still inside the outer comment
  */
  def run(): Unit = {}
}

class Another {
  def execute(): Unit = {}
}
`
	fm, ok := filemap.Generate(src, "nested.scala")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (nested comment '}' corrupted depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "run") {
		t.Errorf("expected 'run' in Example Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "Another") {
		t.Errorf("expected Another as second entry, got: %s", fm.Map[1].Kind)
	}
	// Verify line ranges are not corrupted.
	if !strings.HasPrefix(fm.Map[0].Lines, "1") {
		t.Errorf("Example should start at line 1, got: %s", fm.Map[0].Lines)
	}
	if !strings.HasPrefix(fm.Map[1].Lines, "9") {
		t.Errorf("Another should start at line 9, got: %s", fm.Map[1].Lines)
	}
}

func TestGenerate_Scala_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "App.scala")
	if ok {
		t.Error("expected fallthrough for empty Scala file")
	}
}

func TestGenerate_Scala_FallsThrough_NoStructure(t *testing.T) {
	src := `package com.example

import scala.io.Source
import java.io.File
`
	_, ok := filemap.Generate(src, "package.scala")
	if ok {
		t.Error("expected fallthrough for Scala file with no class/object/trait")
	}
}
