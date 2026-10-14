package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_PHP_ServiceClass(t *testing.T) {
	src := `<?php

declare(strict_types=1);

namespace App\Service;

use App\Repository\UserRepository;
use App\Model\User;

class UserService
{
    public function __construct(
        private readonly UserRepository $repo
    ) {}

    public function createUser(string $email, string $name): User
    {
        $user = new User($email, $name);
        return $this->repo->save($user);
    }

    public function findUser(int $id): ?User
    {
        return $this->repo->findById($id);
    }

    public function deleteUser(int $id): void
    {
        $this->repo->deleteById($id);
    }
}
`
	fm, ok := filemap.Generate(src, "UserService.php")
	if !ok {
		t.Fatal("expected deterministic map for PHP file")
	}
	if !strings.Contains(fm.Summary, "PHP") {
		t.Errorf("expected 'PHP' in summary, got: %s", fm.Summary)
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

func TestGenerate_PHP_Interface(t *testing.T) {
	src := `<?php

namespace App\Repository;

interface UserRepository
{
    public function findById(int $id): ?User;
    public function findAll(): array;
    public function save(User $user): User;
    public function deleteById(int $id): void;
}
`
	fm, ok := filemap.Generate(src, "UserRepository.php")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "interface UserRepository") {
		t.Errorf("expected 'interface UserRepository' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "interface") {
		t.Errorf("expected 'interface' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_PHP_Trait(t *testing.T) {
	src := `<?php

namespace App\Traits;

trait Timestampable
{
    private \DateTime $createdAt;
    private \DateTime $updatedAt;

    public function getCreatedAt(): \DateTime
    {
        return $this->createdAt;
    }

    public function getUpdatedAt(): \DateTime
    {
        return $this->updatedAt;
    }

    public function touch(): void
    {
        $this->updatedAt = new \DateTime();
    }
}
`
	fm, ok := filemap.Generate(src, "Timestampable.php")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "trait Timestampable") {
		t.Errorf("expected 'trait Timestampable' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "trait") {
		t.Errorf("expected 'trait' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_PHP_Enum(t *testing.T) {
	// PHP 8.1 backed enum
	src := `<?php

namespace App\Enum;

enum UserRole: string
{
    case Admin = 'admin';
    case Editor = 'editor';
    case Viewer = 'viewer';

    public function label(): string
    {
        return match($this) {
            UserRole::Admin  => 'Administrator',
            UserRole::Editor => 'Editor',
            UserRole::Viewer => 'Viewer',
        };
    }
}
`
	fm, ok := filemap.Generate(src, "UserRole.php")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "enum UserRole") {
		t.Errorf("expected 'enum UserRole' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "enum") {
		t.Errorf("expected 'enum' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_PHP_AbstractClass(t *testing.T) {
	src := `<?php

namespace App\Controller;

abstract class BaseController
{
    abstract protected function validate(array $data): bool;

    public function respond(mixed $data, int $status = 200): Response
    {
        return new Response(json_encode($data), $status);
    }
}

class UserController extends BaseController
{
    protected function validate(array $data): bool
    {
        return isset($data['email']);
    }

    public function create(Request $request): Response
    {
        $data = $request->json()->all();
        if (!$this->validate($data)) {
            return $this->respond(['error' => 'invalid'], 400);
        }
        return $this->respond(['ok' => true]);
    }
}
`
	fm, ok := filemap.Generate(src, "UserController.php")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "abstract class BaseController") {
		t.Errorf("expected 'abstract class BaseController', got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "class UserController") {
		t.Errorf("expected 'class UserController', got: %s", fm.Map[1].Kind)
	}
	if !strings.Contains(fm.Summary, "abstract class") {
		t.Errorf("expected 'abstract class' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_PHP_AttributedClass(t *testing.T) {
	// PHP 8 #[Attribute] syntax
	src := `<?php

namespace App\Controller;

use Symfony\Component\Routing\Annotation\Route;

#[Route('/api/users')]
class ApiUserController
{
    #[Route('', methods: ['GET'])]
    public function index(): JsonResponse
    {
        return new JsonResponse($this->service->findAll());
    }

    #[Route('/{id}', methods: ['GET'])]
    public function show(int $id): JsonResponse
    {
        return new JsonResponse($this->service->find($id));
    }

    #[Route('', methods: ['POST'])]
    public function store(Request $request): JsonResponse
    {
        $user = $this->service->create($request->toArray());
        return new JsonResponse($user, 201);
    }
}
`
	fm, ok := filemap.Generate(src, "ApiUserController.php")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(fm.Map), fm.Map)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "class ApiUserController") {
		t.Errorf("expected 'class ApiUserController' in Kind, got: %s", kind)
	}
	for _, m := range []string{"index", "show", "store"} {
		if !strings.Contains(kind, m) {
			t.Errorf("expected method %q in Kind, got: %s", m, kind)
		}
	}
}

func TestGenerate_PHP_MethodOverflow(t *testing.T) {
	src := `<?php
class ApiClient
{
    public function get(string $url): array { return []; }
    public function post(string $url, array $data): array { return []; }
    public function put(string $url, array $data): array { return []; }
    public function delete(string $url): void {}
    public function patch(string $url, array $data): array { return []; }
}
`
	fm, ok := filemap.Generate(src, "ApiClient.php")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+1)") {
		t.Errorf("expected (+1) for 5th method, got: %s", kind)
	}
}

// TestGenerate_PHP_BlockComment verifies that braces inside /* ... */ block
// comments do not corrupt depth tracking.
func TestGenerate_PHP_BlockComment(t *testing.T) {
	src := `<?php
class Example
{
    /* } */ public function run(): void {}
}

class Another
{
    public function check(): bool { return true; }
}
`
	fm, ok := filemap.Generate(src, "example.php")
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

// TestGenerate_PHP_Heredoc verifies that braces inside heredoc strings do not
// corrupt depth tracking.
func TestGenerate_PHP_Heredoc(t *testing.T) {
	src := `<?php
class SqlHelper
{
    private string $query = <<<SQL
        SELECT * FROM users WHERE data = '{"role": "admin"}'
        AND status = 'active'
    SQL;

    public function run(\PDO $db): array
    {
        return $db->query($this->query)->fetchAll();
    }
}

class AfterHeredoc
{
    public function check(): bool { return true; }
}
`
	fm, ok := filemap.Generate(src, "SqlHelper.php")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (heredoc brace corrupted depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "run") {
		t.Errorf("expected 'run' in SqlHelper Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "AfterHeredoc") {
		t.Errorf("expected AfterHeredoc as second entry, got: %s", fm.Map[1].Kind)
	}
}

func TestGenerate_PHP_LineRanges(t *testing.T) {
	src := `<?php
class Alpha
{
    public function hello(): void {}
}

class Beta
{
    public function world(): void {}
}
`
	fm, ok := filemap.Generate(src, "ranges.php")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(fm.Map))
	}
	if !strings.HasPrefix(fm.Map[0].Lines, "2") {
		t.Errorf("Alpha should start at line 2, got: %s", fm.Map[0].Lines)
	}
	if !strings.HasPrefix(fm.Map[1].Lines, "7") {
		t.Errorf("Beta should start at line 7, got: %s", fm.Map[1].Lines)
	}
}

func TestGenerate_PHP_GeneratedFile(t *testing.T) {
	src := `<?php
// This file was automatically generated by protoc-gen-php.
// DO NOT EDIT.

namespace App\Proto;

class UserProto
{
    public function getEmail(): string { return $this->email; }
}
`
	fm, ok := filemap.Generate(src, "UserProto.php")
	if !ok {
		t.Fatal("expected deterministic map for generated PHP file")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_PHP_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "app.php")
	if ok {
		t.Error("expected fallthrough for empty PHP file")
	}
}

func TestGenerate_PHP_FallsThrough_NoStructure(t *testing.T) {
	src := `<?php

use App\Foo;
use App\Bar;

$x = new Foo();
echo $x->run();
`
	_, ok := filemap.Generate(src, "script.php")
	if ok {
		t.Error("expected fallthrough for PHP script with no class/interface/trait")
	}
}
