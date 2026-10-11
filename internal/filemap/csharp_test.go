package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_CSharp_ServiceClass_FileScopedNamespace(t *testing.T) {
	src := `using System;
using Microsoft.EntityFrameworkCore;

namespace MyApp.Services;

public class UserService
{
    private readonly AppDbContext _db;

    public UserService(AppDbContext db)
    {
        _db = db;
    }

    public async Task<User> CreateUserAsync(string email)
    {
        var user = new User { Email = email };
        _db.Users.Add(user);
        await _db.SaveChangesAsync();
        return user;
    }

    public User? FindUser(Guid id) => _db.Users.Find(id);

    public async Task DeleteUserAsync(Guid id)
    {
        var user = _db.Users.Find(id);
        if (user != null) _db.Users.Remove(user);
        await _db.SaveChangesAsync();
    }
}
`
	fm, ok := filemap.Generate(src, "UserService.cs")
	if !ok {
		t.Fatal("expected deterministic map for C# file")
	}
	if !strings.Contains(fm.Summary, "C#") {
		t.Errorf("expected 'C#' in summary, got: %s", fm.Summary)
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 class entry, got %d: %v", len(fm.Map), fm.Map)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "class UserService") {
		t.Errorf("expected 'class UserService' in Kind, got: %s", kind)
	}
	for _, m := range []string{"CreateUserAsync", "FindUser", "DeleteUserAsync"} {
		if !strings.Contains(kind, m) {
			t.Errorf("expected method %q in Kind, got: %s", m, kind)
		}
	}
}

func TestGenerate_CSharp_BlockNamespace(t *testing.T) {
	src := `namespace MyApp.Models
{
    public class Post
    {
        public Guid Id { get; set; }
        public string Title { get; set; } = "";

        public string Slug() => Title.ToLower().Replace(" ", "-");
        public bool IsPublished() => PublishedAt.HasValue;
    }

    public class Comment
    {
        public string Body { get; set; } = "";
        public bool IsValid() => Body.Length > 0;
    }
}
`
	fm, ok := filemap.Generate(src, "Models.cs")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (block namespace), got %d: %v", len(fm.Map), fm.Map)
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

func TestGenerate_CSharp_Interface(t *testing.T) {
	src := `namespace MyApp.Contracts;

public interface IUserRepository
{
    Task<User?> FindByIdAsync(Guid id);
    Task<IEnumerable<User>> FindAllAsync();
    Task<User> SaveAsync(User user);
    Task DeleteAsync(Guid id);
}
`
	fm, ok := filemap.Generate(src, "IUserRepository.cs")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "interface IUserRepository") {
		t.Errorf("expected 'interface IUserRepository' in Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "interface") {
		t.Errorf("expected 'interface' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_CSharp_Record(t *testing.T) {
	src := `namespace MyApp.Dtos;

public record UserDto(Guid Id, string Email, string Name)
{
    public string DisplayName() => $"{Name} <{Email}>";
}

public record struct Point(double X, double Y);
`
	fm, ok := filemap.Generate(src, "Dtos.cs")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "record UserDto") {
		t.Errorf("expected 'record UserDto' as first entry, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "record") {
		t.Errorf("expected 'record' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_CSharp_AbstractAndSealedClass(t *testing.T) {
	src := `namespace MyApp.Base;

public abstract class BaseService
{
    protected abstract Task ValidateAsync(object input);
    public async Task ExecuteAsync(object input)
    {
        await ValidateAsync(input);
    }
}

public sealed class FinalHelper
{
    public static string Format(string s) => s.Trim();
}
`
	fm, ok := filemap.Generate(src, "BaseService.cs")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(fm.Map))
	}
	if !strings.Contains(fm.Map[0].Kind, "abstract class BaseService") {
		t.Errorf("expected 'abstract class BaseService', got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "sealed class FinalHelper") {
		t.Errorf("expected 'sealed class FinalHelper', got: %s", fm.Map[1].Kind)
	}
	if !strings.Contains(fm.Summary, "abstract class") {
		t.Errorf("expected 'abstract class' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_CSharp_Enum(t *testing.T) {
	src := `namespace MyApp.Models;

public enum UserRole
{
    Admin,
    Editor,
    Viewer
}
`
	fm, ok := filemap.Generate(src, "UserRole.cs")
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

func TestGenerate_CSharp_AttributedController(t *testing.T) {
	// ASP.NET controllers have [ApiController], [Route], [HttpGet] etc.
	src := `using Microsoft.AspNetCore.Mvc;

namespace MyApp.Controllers;

[ApiController]
[Route("api/[controller]")]
public class UsersController : ControllerBase
{
    [HttpGet]
    public IActionResult GetAll() => Ok(_service.FindAll());

    [HttpGet("{id}")]
    public IActionResult Get(Guid id) => Ok(_service.Find(id));

    [HttpPost]
    public async Task<IActionResult> Create([FromBody] CreateUserDto dto)
    {
        var user = await _service.CreateAsync(dto);
        return CreatedAtAction(nameof(Get), new { id = user.Id }, user);
    }
}
`
	fm, ok := filemap.Generate(src, "UsersController.cs")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(fm.Map), fm.Map)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "class UsersController") {
		t.Errorf("expected 'class UsersController' in Kind, got: %s", kind)
	}
	for _, m := range []string{"GetAll", "Get", "Create"} {
		if !strings.Contains(kind, m) {
			t.Errorf("expected method %q in Kind, got: %s", m, kind)
		}
	}
}

func TestGenerate_CSharp_MethodOverflow(t *testing.T) {
	src := `public class ApiClient
{
    public string Get() => "";
    public string Post() => "";
    public string Put() => "";
    public string Delete() => "";
    public string Patch() => "";
}
`
	fm, ok := filemap.Generate(src, "ApiClient.cs")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+1)") {
		t.Errorf("expected (+1) for 5th method, got: %s", kind)
	}
}

// TestGenerate_CSharp_BlockComment verifies that braces inside /* ... */
// block comments do not corrupt depth tracking.
func TestGenerate_CSharp_BlockComment(t *testing.T) {
	src := `public class Example
{
    /* } */ public void Run() {}
}

public class Another
{
    public bool Check() => true;
}
`
	fm, ok := filemap.Generate(src, "example.cs")
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

// TestGenerate_CSharp_VerbatimString verifies that braces inside @"..."
// verbatim string literals do not corrupt depth tracking.
func TestGenerate_CSharp_VerbatimString(t *testing.T) {
	src := `public class SqlHelper
{
    private const string Query = @"SELECT * FROM users WHERE name = 'foo { bar }'";

    public List<User> Run(DbContext db)
    {
        return db.Users.FromSqlRaw(Query).ToList();
    }
}

public class AfterVerbatim
{
    public void Check() {}
}
`
	fm, ok := filemap.Generate(src, "SqlHelper.cs")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (verbatim string brace corrupted depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "Run") {
		t.Errorf("expected 'Run' in SqlHelper Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "AfterVerbatim") {
		t.Errorf("expected AfterVerbatim as second entry, got: %s", fm.Map[1].Kind)
	}
}

func TestGenerate_CSharp_LineRanges(t *testing.T) {
	src := `class Alpha
{
    void Hello() {}
}

class Beta
{
    void World() {}
}
`
	fm, ok := filemap.Generate(src, "ranges.cs")
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

func TestGenerate_CSharp_GeneratedFile(t *testing.T) {
	src := `// <auto-generated>
//     This code was generated by a tool.
// </auto-generated>
namespace MyApp.Generated;

public partial class UserProto
{
    public static UserProto Parser { get; }
}
`
	fm, ok := filemap.Generate(src, "UserProto.cs")
	if !ok {
		t.Fatal("expected deterministic map for generated C# file")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_CSharp_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "App.cs")
	if ok {
		t.Error("expected fallthrough for empty C# file")
	}
}

func TestGenerate_CSharp_FallsThrough_NoStructure(t *testing.T) {
	src := `using System;
using System.Collections.Generic;
`
	_, ok := filemap.Generate(src, "imports.cs")
	if ok {
		t.Error("expected fallthrough for C# file with no class/struct/interface")
	}
}
