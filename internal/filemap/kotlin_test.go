package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_Kotlin_ClassWithFunctions(t *testing.T) {
	src := `package com.example

class UserService(
    private val repo: UserRepository,
    private val mailer: MailService,
) {
    fun createUser(email: String, name: String): User {
        val user = User(email = email, name = name)
        repo.save(user)
        mailer.sendWelcome(user)
        return user
    }

    fun findUser(id: Long): User? = repo.findById(id)

    fun deleteUser(id: Long) {
        repo.deleteById(id)
    }

    suspend fun fetchFromRemote(id: Long): User {
        return remoteClient.get(id)
    }
}
`
	fm, ok := filemap.Generate(src, "UserService.kt")
	if !ok {
		t.Fatal("expected deterministic map for Kotlin file")
	}
	if !strings.Contains(fm.Summary, "Kotlin") {
		t.Errorf("expected 'Kotlin' in summary, got: %s", fm.Summary)
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 class entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "class UserService") {
		t.Errorf("expected 'class UserService' in Kind, got: %s", kind)
	}
	for _, fn := range []string{"createUser", "findUser", "deleteUser"} {
		if !strings.Contains(kind, fn) {
			t.Errorf("expected function %q in Kind, got: %s", fn, kind)
		}
	}
}

func TestGenerate_Kotlin_DataAndSealedClass(t *testing.T) {
	src := `data class User(
    val id: Long,
    val email: String,
    val name: String,
)

sealed class Result<out T> {
    data class Success<T>(val value: T) : Result<T>()
    data class Failure(val error: Throwable) : Result<Nothing>()
}

interface UserRepository {
    fun findById(id: Long): User?
    fun save(user: User): User
    fun deleteById(id: Long)
}
`
	fm, ok := filemap.Generate(src, "models.kt")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	if !symbolSet["data class User"] {
		t.Errorf("expected 'data class User' in %v", fm.Symbols)
	}
	if !symbolSet["sealed class Result"] {
		t.Errorf("expected 'sealed class Result' in %v", fm.Symbols)
	}
	if !symbolSet["interface UserRepository"] {
		t.Errorf("expected 'interface UserRepository' in %v", fm.Symbols)
	}
}

func TestGenerate_Kotlin_Object(t *testing.T) {
	src := `object UserFactory {
    fun create(email: String): User = User(email = email)
    fun createAdmin(email: String): User = User(email = email, isAdmin = true)
}

object DatabaseConfig {
    val url: String = System.getenv("DATABASE_URL") ?: "jdbc:postgresql://localhost/app"
    val poolSize: Int = 10
}
`
	fm, ok := filemap.Generate(src, "factories.kt")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 object entries, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Summary, "object") {
		t.Errorf("expected 'object' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Kotlin_EnumClass(t *testing.T) {
	src := `enum class UserRole {
    ADMIN,
    EDITOR,
    VIEWER;

    fun canEdit(): Boolean = this == ADMIN || this == EDITOR
}

enum class HttpStatus(val code: Int) {
    OK(200),
    NOT_FOUND(404),
    INTERNAL_ERROR(500);

    fun isError(): Boolean = code >= 400
}
`
	fm, ok := filemap.Generate(src, "enums.kt")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 enum class entries, got %d", len(fm.Map))
	}
	for _, e := range fm.Map {
		if !strings.Contains(e.Kind, "enum class") {
			t.Errorf("expected 'enum class' in Kind, got: %s", e.Kind)
		}
	}
}

func TestGenerate_Kotlin_FunctionOverflow(t *testing.T) {
	src := `class Controller {
    fun get(): String = ""
    fun post(): String = ""
    fun put(): String = ""
    fun delete(): String = ""
    fun patch(): String = ""
}
`
	fm, ok := filemap.Generate(src, "controller.kt")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+1)") {
		t.Errorf("expected (+1) for 5th function, got: %s", kind)
	}
}

func TestGenerate_Kotlin_BraceInString(t *testing.T) {
	// String templates with ${...} must not affect brace depth.
	src := `class Formatter {
    fun formatUser(user: User): String {
        return "User{id=${user.id}, email=${user.email}}"
    }

    fun formatError(msg: String): String = "Error: {$msg}"
}

class Parser {
    fun parse(input: String): Map<String, Any> = emptyMap()
}
`
	fm, ok := filemap.Generate(src, "formatters.kt")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (brace-in-string broke depth?), got %d: %v", len(fm.Map), fm.Map)
	}
}

func TestGenerate_Kotlin_MultilineString(t *testing.T) {
	// Triple-quoted strings spanning multiple lines must not confuse depth tracking.
	src := `class SqlQueries {
    val createTable = """
        CREATE TABLE users (
            id BIGSERIAL PRIMARY KEY,
            email TEXT NOT NULL
        );
    """.trimIndent()

    fun run(db: Database) {
        db.execute(createTable)
    }
}

class Other {
    fun hello(): String = "world"
}
`
	fm, ok := filemap.Generate(src, "queries.kt")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (multiline string broke depth?), got %d: %v", len(fm.Map), fm.Map)
	}
}

func TestGenerate_Kotlin_LineRanges(t *testing.T) {
	src := `class Alpha {
    fun hello() = "hi"
}

class Beta {
    fun world() = "world"
}
`
	fm, ok := filemap.Generate(src, "ranges.kt")
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

func TestGenerate_Kotlin_GeneratedFile(t *testing.T) {
	src := `// Generated by protoc-gen-kotlin. DO NOT EDIT.
package com.example.proto

data class SearchRequest(
    val query: String = "",
    val pageNumber: Int = 0,
)
`
	fm, ok := filemap.Generate(src, "search.kt")
	if !ok {
		t.Fatal("expected deterministic map for generated Kotlin file")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Kotlin_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "app.kt")
	if ok {
		t.Error("expected fallthrough for empty Kotlin file")
	}
}

func TestGenerate_Kotlin_FallsThrough_NoStructure(t *testing.T) {
	// A file with only package/import declarations.
	src := `package com.example

import java.util.Date
import kotlin.collections.List
`
	_, ok := filemap.Generate(src, "imports.kt")
	if ok {
		t.Error("expected fallthrough for Kotlin file with no class/fun definitions")
	}
}

func TestGenerate_Kotlin_KtsScript(t *testing.T) {
	// .kts files (including build.gradle.kts) should be dispatched to the Kotlin parser.
	src := `plugins {
    kotlin("jvm") version "1.9.0"
    application
}

fun greeting(): String = "Hello from build"
`
	// The plugins block has no class/fun at top level that we recognize — may fall through.
	// But if it has a top-level fun, it should produce a map.
	// This test just verifies .kts extension is handled (not rejected).
	_, _ = filemap.Generate(src, "build.gradle.kts")
	// A .kts file with an actual class should produce a map.
	src2 := `class BuildHelper {
    fun clean() { println("cleaning") }
}
`
	fm, ok := filemap.Generate(src2, "helper.kts")
	if !ok {
		t.Fatal("expected deterministic map for .kts file with class")
	}
	if len(fm.Map) != 1 {
		t.Errorf("expected 1 class entry, got %d", len(fm.Map))
	}
}
