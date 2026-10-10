package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_Java_SpringServiceClass(t *testing.T) {
	src := `package com.example.service;

import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
public class UserService {

    private final UserRepository repo;
    private final MailService mailer;

    public UserService(UserRepository repo, MailService mailer) {
        this.repo = repo;
        this.mailer = mailer;
    }

    @Transactional
    public User createUser(String email, String name) {
        User user = new User(email, name);
        repo.save(user);
        mailer.sendWelcome(user);
        return user;
    }

    public Optional<User> findUser(Long id) {
        return repo.findById(id);
    }

    @Transactional
    public void deleteUser(Long id) {
        repo.deleteById(id);
    }
}
`
	fm, ok := filemap.Generate(src, "UserService.java")
	if !ok {
		t.Fatal("expected deterministic map for Java file")
	}
	if !strings.Contains(fm.Summary, "Java") {
		t.Errorf("expected 'Java' in summary, got: %s", fm.Summary)
	}
	if !strings.Contains(fm.Summary, "class") {
		t.Errorf("expected 'class' in summary, got: %s", fm.Summary)
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 class entry, got %d: %v", len(fm.Map), fm.Map)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "class UserService") {
		t.Errorf("expected 'class UserService' in Kind, got: %s", kind)
	}
	for _, method := range []string{"createUser", "findUser", "deleteUser"} {
		if !strings.Contains(kind, method) {
			t.Errorf("expected method %q in Kind, got: %s", method, kind)
		}
	}
}

func TestGenerate_Java_Interface(t *testing.T) {
	src := `package com.example.repository;

import java.util.Optional;
import java.util.List;

public interface UserRepository {
    Optional<User> findById(Long id);
    List<User> findAll();
    User save(User user);
    void deleteById(Long id);
}
`
	fm, ok := filemap.Generate(src, "UserRepository.java")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "interface UserRepository") {
		t.Errorf("expected 'interface UserRepository' in Kind, got: %s", kind)
	}
	if !strings.Contains(kind, "findById") {
		t.Errorf("expected 'findById' in Kind, got: %s", kind)
	}
	if !strings.Contains(fm.Summary, "interface") {
		t.Errorf("expected 'interface' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Java_Enum(t *testing.T) {
	src := `package com.example.model;

public enum UserRole {
    ADMIN,
    EDITOR,
    VIEWER;

    public boolean canEdit() {
        return this == ADMIN || this == EDITOR;
    }

    public boolean isAdmin() {
        return this == ADMIN;
    }
}
`
	fm, ok := filemap.Generate(src, "UserRole.java")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "enum UserRole") {
		t.Errorf("expected 'enum UserRole' in Kind, got: %s", kind)
	}
	if !strings.Contains(kind, "canEdit") {
		t.Errorf("expected 'canEdit' in Kind, got: %s", kind)
	}
	if !strings.Contains(fm.Summary, "enum") {
		t.Errorf("expected 'enum' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Java_Record(t *testing.T) {
	src := `package com.example.dto;

public record UserDTO(Long id, String email, String name) {
    public String displayName() {
        return name + " <" + email + ">";
    }
}
`
	fm, ok := filemap.Generate(src, "UserDTO.java")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "record UserDTO") {
		t.Errorf("expected 'record UserDTO' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "record") {
		t.Errorf("expected 'record' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Java_AbstractClass(t *testing.T) {
	src := `package com.example.base;

public abstract class AbstractUserService {
    protected abstract User loadUser(Long id);

    public User getUser(Long id) {
        User u = loadUser(id);
        if (u == null) {
            throw new UserNotFoundException(id);
        }
        return u;
    }
}
`
	fm, ok := filemap.Generate(src, "AbstractUserService.java")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "abstract class AbstractUserService") {
		t.Errorf("expected 'abstract class AbstractUserService' in Kind, got: %s", kind)
	}
	if !strings.Contains(fm.Summary, "abstract class") {
		t.Errorf("expected 'abstract class' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Java_AnnotationType(t *testing.T) {
	// @interface is Java's annotation type syntax; it must be recognized as a
	// type declaration and not skipped as a standalone annotation line.
	src := `package com.example.annotation;

import java.lang.annotation.*;

@Retention(RetentionPolicy.RUNTIME)
@Target(ElementType.METHOD)
public @interface Audited {
    String value() default "";
}
`
	fm, ok := filemap.Generate(src, "Audited.java")
	if !ok {
		t.Fatal("expected deterministic map for @interface")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "@interface Audited") {
		t.Errorf("expected '@interface Audited' in Kind, got: %s", fm.Map[0].Kind)
	}
}

func TestGenerate_Java_MultipleClasses(t *testing.T) {
	src := `package com.example;

public class Post {
    private String title;

    public String getTitle() { return title; }
    public void setTitle(String t) { this.title = t; }
}

class Comment {
    private String body;

    public String getBody() { return body; }
}
`
	fm, ok := filemap.Generate(src, "Post.java")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	if !symbolSet["class Post"] {
		t.Errorf("expected 'class Post' in symbols: %v", fm.Symbols)
	}
	if !symbolSet["class Comment"] {
		t.Errorf("expected 'class Comment' in symbols: %v", fm.Symbols)
	}
}

func TestGenerate_Java_MethodOverflow(t *testing.T) {
	src := `public class ApiController {
    public String index() { return ""; }
    public String show() { return ""; }
    public String create() { return ""; }
    public String update() { return ""; }
    public String delete() { return ""; }
}
`
	fm, ok := filemap.Generate(src, "ApiController.java")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+1)") {
		t.Errorf("expected (+1) for 5th method, got: %s", kind)
	}
	if !strings.Contains(kind, "index") {
		t.Errorf("expected 'index' in method list, got: %s", kind)
	}
}

// TestGenerate_Java_MultilineHeader is a regression test for the pendingBlock
// pattern: when the class keyword line has no "{", the parser must scan forward
// until the opening brace appears rather than immediately closing the block.
func TestGenerate_Java_MultilineHeader(t *testing.T) {
	src := `package com.example;

public class UserServiceImpl
        extends AbstractUserService
        implements UserService, AuditableService {

    public User loadUser(Long id) {
        return repo.findById(id).orElseThrow();
    }

    public void auditAccess(Long id) {
        log.info("access: {}", id);
    }
}
`
	fm, ok := filemap.Generate(src, "UserServiceImpl.java")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry (multiline header broke depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "UserServiceImpl") {
		t.Errorf("expected 'UserServiceImpl' in Kind, got: %s", kind)
	}
	if !strings.Contains(kind, "loadUser") {
		t.Errorf("expected 'loadUser' in Kind, got: %s", kind)
	}
}

// TestGenerate_Java_CharLiteral verifies that character literals containing
// braces do not corrupt block depth tracking.
func TestGenerate_Java_CharLiteral(t *testing.T) {
	src := `public class BraceParser {
    private char open  = '{';
    private char close = '}';
    private char tab   = '\t';

    public String wrap(String s) {
        return open + s + close;
    }
}

public class AfterCharLiteral {
    public boolean check() { return true; }
}
`
	fm, ok := filemap.Generate(src, "BraceParser.java")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (char literal '}' corrupted depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "wrap") {
		t.Errorf("expected 'wrap' in BraceParser Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "AfterCharLiteral") {
		t.Errorf("expected AfterCharLiteral as second entry, got: %s", fm.Map[1].Kind)
	}
}

func TestGenerate_Java_LineRanges(t *testing.T) {
	src := `class Alpha {
    public void hello() {}
}

class Beta {
    public void world() {}
}
`
	fm, ok := filemap.Generate(src, "ranges.java")
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

func TestGenerate_Java_GeneratedFile(t *testing.T) {
	src := `// Code generated by protoc-gen-java. DO NOT EDIT.
package com.example.proto;

public final class SearchProto {
    public static com.google.protobuf.Descriptors.FileDescriptor getDescriptor() {
        return descriptor;
    }
}
`
	fm, ok := filemap.Generate(src, "SearchProto.java")
	if !ok {
		t.Fatal("expected deterministic map for generated Java file")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Java_StaticInitBlock(t *testing.T) {
	// static { ... } must not be treated as a method named "static".
	src := `public class Config {
    private static final Map<String, String> DEFAULTS;

    static {
        DEFAULTS = new HashMap<>();
        DEFAULTS.put("host", "localhost");
        DEFAULTS.put("port", "5432");
    }

    public String get(String key) {
        return DEFAULTS.getOrDefault(key, "");
    }
}
`
	fm, ok := filemap.Generate(src, "Config.java")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	// "get" should appear; "static" must NOT appear as a method name.
	if !strings.Contains(kind, "get") {
		t.Errorf("expected 'get' in Kind, got: %s", kind)
	}
	if strings.Contains(kind, "— static") || strings.Contains(kind, ", static") {
		t.Errorf("static initializer block falsely detected as method: %s", kind)
	}
}

// TestGenerate_Java_BlockComment_InlineBrace is the regression test for the
// bug where javaNetBraces did not handle /* ... */ block comments and would
// count the "}" inside "/* } */" as a real closing brace, prematurely ending
// the tracked block. The test also exercises a multiline block comment whose
// opening is mid-line.
func TestGenerate_Java_BlockComment_InlineBrace(t *testing.T) {
	// Inline block comment: "/* } */" on the same line as code.
	src := `class Example {
    /* } */ void run() {}
    class Nested {}
}
class Another {
    public void check() {}
}
`
	fm, ok := filemap.Generate(src, "Example.java")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (block-comment '}' corrupted depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "Example") {
		t.Errorf("first entry should be Example, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "Another") {
		t.Errorf("second entry should be Another, got: %s", fm.Map[1].Kind)
	}

	// Multiline block comment opened mid-line.
	src2 := `class Alpha {
    int x = 1; /* start of
       multiline comment with } brace
    */
    public void hello() {}
}
class Beta {
    public void world() {}
}
`
	fm2, ok2 := filemap.Generate(src2, "Multi.java")
	if !ok2 {
		t.Fatal("expected deterministic map for multiline block comment")
	}
	if len(fm2.Map) != 2 {
		t.Fatalf("expected 2 entries (mid-line block comment broke depth?), got %d: %v", len(fm2.Map), fm2.Map)
	}
	if !strings.Contains(fm2.Map[0].Kind, "hello") {
		t.Errorf("expected 'hello' in Alpha Kind, got: %s", fm2.Map[0].Kind)
	}
	if !strings.Contains(fm2.Map[1].Kind, "Beta") {
		t.Errorf("second entry should be Beta, got: %s", fm2.Map[1].Kind)
	}
}

func TestGenerate_Java_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "App.java")
	if ok {
		t.Error("expected fallthrough for empty Java file")
	}
}

func TestGenerate_Java_FallsThrough_NoStructure(t *testing.T) {
	src := `package com.example;

import java.util.List;
import java.util.Map;
`
	_, ok := filemap.Generate(src, "imports.java")
	if ok {
		t.Error("expected fallthrough for Java file with no class/interface/enum")
	}
}
