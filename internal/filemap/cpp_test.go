package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_Cpp_ClassWithMethods(t *testing.T) {
	src := `#include <string>
#include <optional>

class UserService {
public:
    explicit UserService(UserRepository* repo);

    User createUser(const std::string& email, const std::string& name);
    std::optional<User> findUser(long id);
    void deleteUser(long id);

private:
    UserRepository* repo_;
};
`
	fm, ok := filemap.Generate(src, "UserService.cpp")
	if !ok {
		t.Fatal("expected deterministic map for C++ file")
	}
	if !strings.Contains(fm.Summary, "C/C++") {
		t.Errorf("expected 'C/C++' in summary, got: %s", fm.Summary)
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 class entry, got %d: %v", len(fm.Map), fm.Map)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "class UserService") {
		t.Errorf("expected 'class UserService' in Kind, got: %s", kind)
	}
	for _, m := range []string{"createUser", "findUser", "deleteUser"} {
		if !strings.Contains(kind, m) {
			t.Errorf("expected method %q in Kind, got: %s", m, kind)
		}
	}
}

func TestGenerate_Cpp_Struct(t *testing.T) {
	src := `struct Point {
    double x;
    double y;

    double distance(const Point& other) const;
    bool operator==(const Point& other) const;
};
`
	fm, ok := filemap.Generate(src, "Point.h")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 struct entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "struct Point") {
		t.Errorf("expected 'struct Point' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[0].Kind, "distance") {
		t.Errorf("expected 'distance' method in Kind, got: %s", fm.Map[0].Kind)
	}
}

func TestGenerate_Cpp_EnumClass(t *testing.T) {
	src := `enum class UserRole {
    Admin,
    Editor,
    Viewer
};
`
	fm, ok := filemap.Generate(src, "UserRole.h")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 enum class entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "enum class UserRole") {
		t.Errorf("expected 'enum class UserRole' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "enum") {
		t.Errorf("expected 'enum' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Cpp_Namespace(t *testing.T) {
	src := `namespace myapp {

class Logger {
public:
    void info(const std::string& msg);
    void warn(const std::string& msg);
    void error(const std::string& msg);
};

struct Config {
    std::string host;
    int port;

    bool isValid() const;
};

} // namespace myapp
`
	fm, ok := filemap.Generate(src, "logger.h")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (namespace transparent), got %d: %v", len(fm.Map), fm.Map)
	}
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	if !symbolSet["class Logger"] {
		t.Errorf("expected 'class Logger' in symbols: %v", fm.Symbols)
	}
	if !symbolSet["struct Config"] {
		t.Errorf("expected 'struct Config' in symbols: %v", fm.Symbols)
	}
}

func TestGenerate_Cpp_MultipleClasses(t *testing.T) {
	src := `class Animal {
public:
    virtual std::string speak() const = 0;
    virtual ~Animal() = default;
};

class Dog : public Animal {
public:
    std::string speak() const override;
    void fetch(const std::string& item);
};

class Cat : public Animal {
public:
    std::string speak() const override;
    void purr();
};
`
	fm, ok := filemap.Generate(src, "animals.h")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "class Animal") {
		t.Errorf("expected 'class Animal', got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "Dog") {
		t.Errorf("expected Dog as second entry, got: %s", fm.Map[1].Kind)
	}
	if !strings.Contains(fm.Map[2].Kind, "Cat") {
		t.Errorf("expected Cat as third entry, got: %s", fm.Map[2].Kind)
	}
}

func TestGenerate_Cpp_MethodOverflow(t *testing.T) {
	src := `class ApiClient {
public:
    std::string get(const std::string& url);
    std::string post(const std::string& url, const std::string& body);
    std::string put(const std::string& url, const std::string& body);
    std::string del(const std::string& url);
    std::string patch(const std::string& url, const std::string& body);
};
`
	fm, ok := filemap.Generate(src, "ApiClient.h")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+1)") {
		t.Errorf("expected (+1) for 5th method, got: %s", kind)
	}
}

// TestGenerate_Cpp_BlockComment verifies that braces inside /* ... */ block
// comments, including mid-line, do not corrupt depth tracking.
func TestGenerate_Cpp_BlockComment(t *testing.T) {
	src := `class Example {
public:
    /* } */ void run() {}
};

class Another {
public:
    bool check() { return true; }
};
`
	fm, ok := filemap.Generate(src, "example.h")
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

// TestGenerate_Cpp_CharLiteral verifies that character literals containing
// braces do not corrupt depth tracking.
func TestGenerate_Cpp_CharLiteral(t *testing.T) {
	src := `class BraceScanner {
public:
    static const char kOpen  = '{';
    static const char kClose = '}';

    bool isOpen(char c) { return c == '{'; }
};

class AfterCharLiteral {
public:
    void check() {}
};
`
	fm, ok := filemap.Generate(src, "scanner.h")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (char literal '}' corrupted depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[1].Kind, "AfterCharLiteral") {
		t.Errorf("expected AfterCharLiteral as second entry, got: %s", fm.Map[1].Kind)
	}
}

// TestGenerate_Cpp_RawString verifies that braces inside R"(...)" raw strings
// do not corrupt depth tracking.
func TestGenerate_Cpp_RawString(t *testing.T) {
	src := `class SqlHelper {
public:
    static constexpr const char* kQuery = R"(SELECT * FROM users WHERE data = '{"role":"admin"}')";

    std::vector<User> run(Database& db);
};

class AfterRawString {
public:
    void check() {}
};
`
	fm, ok := filemap.Generate(src, "sql.h")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (raw string brace corrupted depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "run") {
		t.Errorf("expected 'run' in SqlHelper Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "AfterRawString") {
		t.Errorf("expected AfterRawString as second entry, got: %s", fm.Map[1].Kind)
	}
}

func TestGenerate_Cpp_HeaderGuard(t *testing.T) {
	src := `#ifndef MYAPP_USER_H
#define MYAPP_USER_H

#include <string>

class User {
public:
    explicit User(std::string email);
    std::string getEmail() const;
    bool isActive() const;

private:
    std::string email_;
    bool active_ = true;
};

#endif // MYAPP_USER_H
`
	fm, ok := filemap.Generate(src, "user.h")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(fm.Map), fm.Map)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "class User") {
		t.Errorf("expected 'class User' in Kind, got: %s", kind)
	}
	if !strings.Contains(kind, "getEmail") {
		t.Errorf("expected 'getEmail' in Kind, got: %s", kind)
	}
}

func TestGenerate_Cpp_LineRanges(t *testing.T) {
	src := `class Alpha {
public:
    void hello();
};

class Beta {
public:
    void world();
};
`
	fm, ok := filemap.Generate(src, "ranges.h")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(fm.Map))
	}
	if !strings.HasPrefix(fm.Map[0].Lines, "1") {
		t.Errorf("Alpha should start at line 1, got: %s", fm.Map[0].Lines)
	}
	if !strings.HasPrefix(fm.Map[1].Lines, "6") {
		t.Errorf("Beta should start at line 6, got: %s", fm.Map[1].Lines)
	}
}

func TestGenerate_Cpp_GeneratedFile(t *testing.T) {
	src := `// DO NOT EDIT. Generated by protoc-gen-cpp.
#include "user.pb.h"

class UserProto {
public:
    static const std::string& default_instance();
};
`
	fm, ok := filemap.Generate(src, "user.pb.cpp")
	if !ok {
		t.Fatal("expected deterministic map for generated C++ file")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Cpp_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "app.cpp")
	if ok {
		t.Error("expected fallthrough for empty C++ file")
	}
}

func TestGenerate_Cpp_FallsThrough_NoStructure(t *testing.T) {
	src := `#include <iostream>
#include <string>

int main() {
    std::cout << "hello world" << std::endl;
    return 0;
}
`
	_, ok := filemap.Generate(src, "main.cpp")
	if ok {
		t.Error("expected fallthrough for C++ file with no class/struct/enum")
	}
}

func TestGenerate_Cpp_CFile(t *testing.T) {
	// Pure C file — struct and enum, no class.
	src := `#include <stdint.h>

typedef struct {
    uint8_t r;
    uint8_t g;
    uint8_t b;
    uint8_t a;
} Color;

typedef enum {
    LOG_DEBUG,
    LOG_INFO,
    LOG_WARN,
    LOG_ERROR
} LogLevel;
`
	fm, ok := filemap.Generate(src, "types.h")
	if !ok {
		t.Fatal("expected deterministic map for C header")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (struct + enum), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Summary, "C/C++") {
		t.Errorf("expected 'C/C++' in summary, got: %s", fm.Summary)
	}
}
