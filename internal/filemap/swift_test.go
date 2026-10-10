package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_Swift_ClassWithMethods(t *testing.T) {
	src := `import UIKit

class UserViewController: UIViewController {
    var user: User?

    override func viewDidLoad() {
        super.viewDidLoad()
        loadUser()
    }

    private func loadUser() {
        guard let id = userId else { return }
        viewModel.fetchUser(id: id)
    }

    @IBAction func didTapSave(_ sender: UIButton) {
        viewModel.saveUser()
    }
}
`
	fm, ok := filemap.Generate(src, "UserViewController.swift")
	if !ok {
		t.Fatal("expected deterministic map for Swift file")
	}
	if !strings.Contains(fm.Summary, "Swift") {
		t.Errorf("expected 'Swift' in summary, got: %s", fm.Summary)
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 class entry, got %d: %v", len(fm.Map), fm.Map)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "class UserViewController") {
		t.Errorf("expected 'class UserViewController' in Kind, got: %s", kind)
	}
	for _, fn := range []string{"viewDidLoad", "loadUser", "didTapSave"} {
		if !strings.Contains(kind, fn) {
			t.Errorf("expected func %q in Kind, got: %s", fn, kind)
		}
	}
}

func TestGenerate_Swift_Struct(t *testing.T) {
	src := `struct User: Codable {
    let id: UUID
    let email: String
    let name: String

    func displayName() -> String {
        return "\(name) <\(email)>"
    }

    static func from(dto: UserDTO) -> User {
        return User(id: dto.id, email: dto.email, name: dto.name)
    }
}
`
	fm, ok := filemap.Generate(src, "User.swift")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 struct entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "struct User") {
		t.Errorf("expected 'struct User' in Kind, got: %s", kind)
	}
	for _, fn := range []string{"displayName", "from"} {
		if !strings.Contains(kind, fn) {
			t.Errorf("expected func %q in Kind, got: %s", kind, fn)
		}
	}
	if !strings.Contains(fm.Summary, "struct") {
		t.Errorf("expected 'struct' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Swift_Protocol(t *testing.T) {
	src := `protocol UserRepository {
    func findById(_ id: UUID) -> User?
    func save(_ user: User)
    func deleteById(_ id: UUID)
    func findAll() -> [User]
}
`
	fm, ok := filemap.Generate(src, "UserRepository.swift")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 protocol entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "protocol UserRepository") {
		t.Errorf("expected 'protocol UserRepository' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "protocol") {
		t.Errorf("expected 'protocol' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Swift_Enum(t *testing.T) {
	src := `enum UserRole: String, Codable {
    case admin
    case editor
    case viewer

    var canEdit: Bool {
        return self == .admin || self == .editor
    }

    func description() -> String {
        return rawValue.capitalized
    }
}
`
	fm, ok := filemap.Generate(src, "UserRole.swift")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 enum entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "enum UserRole") {
		t.Errorf("expected 'enum UserRole' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[0].Kind, "description") {
		t.Errorf("expected 'description' func in Kind, got: %s", fm.Map[0].Kind)
	}
}

func TestGenerate_Swift_Extension(t *testing.T) {
	src := `class UserService {
    let repo: UserRepository

    init(repo: UserRepository) {
        self.repo = repo
    }

    func createUser(email: String) -> User {
        let user = User(email: email)
        repo.save(user)
        return user
    }
}

extension UserService: CustomStringConvertible {
    var description: String {
        return "UserService"
    }
}
`
	fm, ok := filemap.Generate(src, "UserService.swift")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (class + extension), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "class UserService") {
		t.Errorf("expected 'class UserService' as first entry, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[0].Kind, "init") {
		t.Errorf("expected 'init' in class Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "extension UserService") {
		t.Errorf("expected 'extension UserService' as second entry, got: %s", fm.Map[1].Kind)
	}
}

func TestGenerate_Swift_Actor(t *testing.T) {
	src := `actor UserCache {
    private var cache: [UUID: User] = [:]

    func get(_ id: UUID) -> User? {
        return cache[id]
    }

    func set(_ user: User) {
        cache[user.id] = user
    }

    func invalidate(_ id: UUID) {
        cache.removeValue(forKey: id)
    }
}
`
	fm, ok := filemap.Generate(src, "UserCache.swift")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 actor entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "actor UserCache") {
		t.Errorf("expected 'actor UserCache' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "actor") {
		t.Errorf("expected 'actor' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Swift_FuncOverflow(t *testing.T) {
	src := `class ApiClient {
    func get() {}
    func post() {}
    func put() {}
    func delete() {}
    func patch() {}
}
`
	fm, ok := filemap.Generate(src, "ApiClient.swift")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if !strings.Contains(fm.Map[0].Kind, "(+1)") {
		t.Errorf("expected (+1) for 5th func, got: %s", fm.Map[0].Kind)
	}
}

// TestGenerate_Swift_BlockComment verifies that braces inside /* ... */ block
// comments, including inline comments, do not corrupt depth tracking.
func TestGenerate_Swift_BlockComment(t *testing.T) {
	src := `class Example {
    /* } */ func run() {}
    struct Nested {}
}
class Another {
    func check() -> Bool { return true }
}
`
	fm, ok := filemap.Generate(src, "example.swift")
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

// TestGenerate_Swift_TripleQuote verifies that braces inside triple-quoted
// strings do not affect depth tracking.
func TestGenerate_Swift_TripleQuote(t *testing.T) {
	src := `struct SqlQueries {
    let createTable = """
        CREATE TABLE users (
            id UUID PRIMARY KEY,
            email TEXT NOT NULL
        );
    """

    func run(db: Database) {
        db.execute(createTable)
    }
}

struct Other {
    func hello() -> String { return "world" }
}
`
	fm, ok := filemap.Generate(src, "queries.swift")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (triple-quote broke depth?), got %d: %v", len(fm.Map), fm.Map)
	}
}

func TestGenerate_Swift_LineRanges(t *testing.T) {
	src := `class Alpha {
    func hello() {}
}

class Beta {
    func world() {}
}
`
	fm, ok := filemap.Generate(src, "ranges.swift")
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

func TestGenerate_Swift_GeneratedFile(t *testing.T) {
	src := `// Generated by SwiftGen. DO NOT EDIT.
import Foundation

enum Asset {
    enum Colors {
        static let accent = ColorAsset(name: "Accent")
    }
}
`
	fm, ok := filemap.Generate(src, "Assets.swift")
	if !ok {
		t.Fatal("expected deterministic map for generated Swift file")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Swift_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "App.swift")
	if ok {
		t.Error("expected fallthrough for empty Swift file")
	}
}

func TestGenerate_Swift_FallsThrough_NoStructure(t *testing.T) {
	src := `import Foundation
import UIKit

let version = "1.0.0"
`
	_, ok := filemap.Generate(src, "constants.swift")
	if ok {
		t.Error("expected fallthrough for Swift file with no class/struct/enum")
	}
}

func TestGenerate_Swift_FinalClass(t *testing.T) {
	src := `public final class UserManager {
    static let shared = UserManager()

    private init() {}

    func currentUser() -> User? { return nil }
}
`
	fm, ok := filemap.Generate(src, "UserManager.swift")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "final class UserManager") {
		t.Errorf("expected 'final class UserManager' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[0].Kind, "init") {
		t.Errorf("expected 'init' in Kind, got: %s", fm.Map[0].Kind)
	}
}
