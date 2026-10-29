package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_Dart_StatelessWidget(t *testing.T) {
	src := `import 'package:flutter/material.dart';

class UserCard extends StatelessWidget {
  const UserCard({super.key, required this.user});

  final User user;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: ListTile(
        title: Text(user.name),
        subtitle: Text(user.email),
      ),
    );
  }

  void _handleTap() {
    debugPrint('tapped: ${user.id}');
  }

  String get displayName => user.name;
}
`
	fm, ok := filemap.Generate(src, "user_card.dart")
	if !ok {
		t.Fatal("expected deterministic map for Dart file")
	}
	if !strings.Contains(fm.Summary, "Dart") {
		t.Errorf("expected 'Dart' in summary, got: %s", fm.Summary)
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(fm.Map), fm.Map)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "class UserCard") {
		t.Errorf("expected 'class UserCard' in Kind, got: %s", kind)
	}
	if !strings.Contains(kind, "build") {
		t.Errorf("expected 'build' in Kind, got: %s", kind)
	}
}

func TestGenerate_Dart_StatefulWidget(t *testing.T) {
	src := `import 'package:flutter/material.dart';

class CounterWidget extends StatefulWidget {
  const CounterWidget({super.key});

  @override
  State<CounterWidget> createState() => _CounterWidgetState();
}

class _CounterWidgetState extends State<CounterWidget> {
  int _count = 0;

  void _increment() {
    setState(() => _count++);
  }

  void _decrement() {
    setState(() => _count--);
  }

  void _reset() {
    setState(() => _count = 0);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(child: Text('$_count')),
    );
  }
}
`
	fm, ok := filemap.Generate(src, "counter.dart")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "class CounterWidget") {
		t.Errorf("expected 'class CounterWidget', got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "class _CounterWidgetState") {
		t.Errorf("expected 'class _CounterWidgetState', got: %s", fm.Map[1].Kind)
	}
	for _, m := range []string{"build", "_increment", "_decrement"} {
		if !strings.Contains(fm.Map[1].Kind, m) {
			t.Errorf("expected %q in _CounterWidgetState Kind, got: %s", m, fm.Map[1].Kind)
		}
	}
}

func TestGenerate_Dart_AbstractClass(t *testing.T) {
	src := `abstract class Repository<T> {
  Future<T?> findById(String id);
  Future<List<T>> findAll();
  Future<T> save(T entity);
  Future<void> delete(String id);
}

class UserRepository extends Repository<User> {
  @override
  Future<User?> findById(String id) async {
    return _db.findUser(id);
  }

  @override
  Future<List<User>> findAll() async {
    return _db.listUsers();
  }

  @override
  Future<User> save(User user) async {
    return _db.saveUser(user);
  }

  @override
  Future<void> delete(String id) async {
    _db.deleteUser(id);
  }
}
`
	fm, ok := filemap.Generate(src, "repository.dart")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "abstract class Repository") {
		t.Errorf("expected 'abstract class Repository', got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "abstract class") {
		t.Errorf("expected 'abstract class' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Dart_Mixin(t *testing.T) {
	src := `mixin Validatable {
  Map<String, String> get errors;

  bool validate();

  bool get isValid => validate();
}

mixin class Loggable {
  void log(String message) {
    debugPrint('[${runtimeType}] $message');
  }

  void logError(String message, [Object? error]) {
    debugPrint('[${runtimeType}] ERROR: $message ${error ?? ""}');
  }
}
`
	fm, ok := filemap.Generate(src, "mixins.dart")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "mixin Validatable") {
		t.Errorf("expected 'mixin Validatable', got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "mixin class Loggable") {
		t.Errorf("expected 'mixin class Loggable', got: %s", fm.Map[1].Kind)
	}
	if !strings.Contains(fm.Summary, "mixin class") || !strings.Contains(fm.Summary, "mixin") {
		t.Errorf("expected both 'mixin' and 'mixin class' in summary, got: %s", fm.Summary)
	}
	for _, m := range []string{"log", "logError"} {
		if !strings.Contains(fm.Map[1].Kind, m) {
			t.Errorf("expected %q in Loggable Kind, got: %s", m, fm.Map[1].Kind)
		}
	}
}

func TestGenerate_Dart_Enum(t *testing.T) {
	// Dart 2.17 enhanced enum with methods.
	src := `enum UserRole {
  admin,
  editor,
  viewer;

  String get displayName => switch (this) {
    UserRole.admin  => 'Administrator',
    UserRole.editor => 'Editor',
    UserRole.viewer => 'Viewer',
  };

  bool get canWrite => this == UserRole.admin || this == UserRole.editor;

  bool hasPermission(Permission p) {
    return p.requiredRole.index <= index;
  }
}
`
	fm, ok := filemap.Generate(src, "user_role.dart")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "enum UserRole") {
		t.Errorf("expected 'enum UserRole', got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "enum") {
		t.Errorf("expected 'enum' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Dart_Extension(t *testing.T) {
	src := `extension StringX on String {
  String get capitalized =>
      isEmpty ? this : '${this[0].toUpperCase()}${substring(1)}';

  String truncate(int maxLength, {String suffix = '...'}) {
    if (length <= maxLength) return this;
    return '${substring(0, maxLength)}$suffix';
  }

  bool get isEmail => contains('@') && contains('.');
}
`
	fm, ok := filemap.Generate(src, "string_ext.dart")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "extension StringX") {
		t.Errorf("expected 'extension StringX', got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Summary, "extension") {
		t.Errorf("expected 'extension' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Dart_Dart3Modifiers(t *testing.T) {
	src := `sealed class Shape {
  double area();
}

final class Circle extends Shape {
  const Circle(this.radius);
  final double radius;

  @override
  double area() => 3.14159 * radius * radius;
}

base class Polygon extends Shape {
  const Polygon(this.sides);
  final int sides;

  @override
  double area() => 0.0;
}
`
	fm, ok := filemap.Generate(src, "shapes.dart")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "sealed class Shape") {
		t.Errorf("expected 'sealed class Shape', got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "final class Circle") {
		t.Errorf("expected 'final class Circle', got: %s", fm.Map[1].Kind)
	}
	if !strings.Contains(fm.Map[2].Kind, "base class Polygon") {
		t.Errorf("expected 'base class Polygon', got: %s", fm.Map[2].Kind)
	}
	if !strings.Contains(fm.Summary, "sealed class") {
		t.Errorf("expected 'sealed class' in summary, got: %s", fm.Summary)
	}
}

func TestGenerate_Dart_MethodOverflow(t *testing.T) {
	src := `class ApiClient {
  Future<Map<String, dynamic>> get(String path) async => {};
  Future<Map<String, dynamic>> post(String path, Map<String, dynamic> body) async => {};
  Future<Map<String, dynamic>> put(String path, Map<String, dynamic> body) async => {};
  Future<void> delete(String path) async {}
  Future<Map<String, dynamic>> patch(String path, Map<String, dynamic> body) async => {};
}
`
	fm, ok := filemap.Generate(src, "api_client.dart")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "(+1)") {
		t.Errorf("expected (+1) for 5th method, got: %s", kind)
	}
}

// TestGenerate_Dart_BlockComment verifies that braces inside /* ... */ comments
// do not corrupt depth tracking. The method on the comment line is not captured
// because the method regex (anchored at ^) cannot skip a leading comment — this
// is the same limitation as other parsers and is acceptable for a file map.
func TestGenerate_Dart_BlockComment(t *testing.T) {
	src := `class Example {
  /* } */ void run() {}
  void compute() {}
}

class Another {
  void check() {}
}
`
	fm, ok := filemap.Generate(src, "example.dart")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (block comment '}' corrupted depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	// compute() is on its own line so it is captured; run() is behind a leading
	// block comment on the same line so it is not (anchored regex limitation).
	if !strings.Contains(fm.Map[0].Kind, "compute") {
		t.Errorf("expected 'compute' in Example Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "Another") {
		t.Errorf("expected Another as second entry, got: %s", fm.Map[1].Kind)
	}
}

// TestGenerate_Dart_TripleQuote verifies that braces inside triple-quoted strings
// do not corrupt depth tracking. Dart triple-quoted strings (""" and ”') are
// handled by the natural toggle behaviour of the persistent *inDouble / *inSingle
// state: three quote characters flip the state an odd number of times, so after
// the opening triple the state is "inside string" and braces are skipped.
func TestGenerate_Dart_TripleQuote(t *testing.T) {
	src := `class SqlStore {
  static const String createTable = """
    CREATE TABLE users (
      id TEXT PRIMARY KEY,
      data JSON NOT NULL DEFAULT '{}'
    )
  """;

  Future<void> initialize(Database db) async {
    await db.execute(createTable);
  }
}

class AfterTriple {
  void run() {}
}
`
	fm, ok := filemap.Generate(src, "sql_store.dart")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (triple-quote broke depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "initialize") {
		t.Errorf("expected 'initialize' in SqlStore Kind, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "AfterTriple") {
		t.Errorf("expected AfterTriple as second entry, got: %s", fm.Map[1].Kind)
	}
}

func TestGenerate_Dart_MultilineString(t *testing.T) {
	src := `class Template {
  final String schema = "
    {
      required: true,
      type: 'object'
    }
  ";

  bool validate(dynamic value) => value != null;
  String render() => schema;
}

class After {
  void run() {}
}
`
	fm, ok := filemap.Generate(src, "template.dart")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries (multiline string braces corrupted depth?), got %d: %v", len(fm.Map), fm.Map)
	}
	for _, m := range []string{"validate", "render"} {
		if !strings.Contains(fm.Map[0].Kind, m) {
			t.Errorf("expected method %q in Template Kind, got: %s", m, fm.Map[0].Kind)
		}
	}
	if !strings.Contains(fm.Map[1].Kind, "After") {
		t.Errorf("expected After as second entry, got: %s", fm.Map[1].Kind)
	}
}

func TestGenerate_Dart_MultilineHeader(t *testing.T) {
	// Class header spanning multiple lines (extends/with/implements).
	src := `class UserBloc
    extends Bloc<UserEvent, UserState>
    with UserValidation
    implements Disposable {

  UserBloc(UserRepository repo) : super(UserInitial()) {
    on<LoadUser>(_onLoadUser);
    on<UpdateUser>(_onUpdateUser);
  }

  Future<void> _onLoadUser(LoadUser event, Emitter<UserState> emit) async {
    emit(UserLoading());
  }

  Future<void> _onUpdateUser(UpdateUser event, Emitter<UserState> emit) async {
    emit(UserLoading());
  }
}
`
	fm, ok := filemap.Generate(src, "user_bloc.dart")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "class UserBloc") {
		t.Errorf("expected 'class UserBloc', got: %s", fm.Map[0].Kind)
	}
}

func TestGenerate_Dart_LineRanges(t *testing.T) {
	src := `class Alpha {
  void hello() {}
}

class Beta {
  void world() {}
}
`
	fm, ok := filemap.Generate(src, "ranges.dart")
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

func TestGenerate_Dart_GeneratedFile(t *testing.T) {
	src := `// GENERATED CODE - DO NOT EDIT
// ignore_for_file: type=lint

part of 'user.dart';

class User {
  final String id;
  final String email;

  User({required this.id, required this.email});

  Map<String, dynamic> toJson() => {'id': id, 'email': email};
}
`
	fm, ok := filemap.Generate(src, "user.g.dart")
	if !ok {
		t.Fatal("expected deterministic map for generated Dart file")
	}
	if !strings.Contains(fm.Summary, "generated") {
		t.Errorf("expected 'generated' in summary, got: %s", fm.Summary)
	}
}

// TestGenerate_Dart_UnnamedExtension verifies that unnamed extension declarations
// (extension on String { ... }) produce a deterministic map. Previously
// dartTopBlock required a name, so these fell through without a map.
func TestGenerate_Dart_UnnamedExtension(t *testing.T) {
	src := `extension on String {
  String get capitalized =>
      isEmpty ? this : '${this[0].toUpperCase()}${substring(1)}';

  String truncate(int max, {String suffix = '...'}) {
    if (length <= max) return this;
    return '${substring(0, max)}$suffix';
  }
}
`
	fm, ok := filemap.Generate(src, "string_ext.dart")
	if !ok {
		t.Fatal("expected deterministic map for unnamed extension (fell through?)")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(fm.Map), fm.Map)
	}
	kind := fm.Map[0].Kind
	if !strings.Contains(kind, "extension on String") {
		t.Errorf("expected 'extension on String' in Kind, got: %s", kind)
	}
	if !strings.Contains(kind, "truncate") {
		t.Errorf("expected 'truncate' in Kind, got: %s", kind)
	}
}

// TestGenerate_Dart_UnnamedExtensionOnlyFile verifies hasDartStructure works for
// a file containing only unnamed extensions (no class/mixin/enum).
func TestGenerate_Dart_UnnamedExtensionOnlyFile(t *testing.T) {
	src := `extension on int {
  bool get isEven => this % 2 == 0;
  bool get isOdd => !isEven;
  int clamp(int min, int max) => this < min ? min : this > max ? max : this;
}

extension on List<int> {
  int get sum => fold(0, (a, b) => a + b);
  double get mean => isEmpty ? 0 : sum / length;
}
`
	fm, ok := filemap.Generate(src, "number_ext.dart")
	if !ok {
		t.Fatal("expected deterministic map — unnamed-extension-only file fell through")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(fm.Map), fm.Map)
	}
	if !strings.Contains(fm.Map[0].Kind, "extension on int") {
		t.Errorf("expected 'extension on int', got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "extension on List") {
		t.Errorf("expected 'extension on List', got: %s", fm.Map[1].Kind)
	}
}

// TestGenerate_Dart_Getters verifies that getter declarations are collected
// in the Kind label. Previously dartMethod required '(' after the name and
// never captured arrow-style getters (String get name => ...).
func TestGenerate_Dart_Getters(t *testing.T) {
	src := `class User {
  final String _firstName;
  final String _lastName;
  final String _email;

  const User(this._firstName, this._lastName, this._email);

  String get firstName => _firstName;
  String get lastName => _lastName;
  String get fullName => '$_firstName $_lastName';
  String get email => _email;
  bool get isValid => _email.contains('@');
}
`
	fm, ok := filemap.Generate(src, "user.dart")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(fm.Map))
	}
	kind := fm.Map[0].Kind
	for _, g := range []string{"firstName", "lastName", "fullName"} {
		if !strings.Contains(kind, g) {
			t.Errorf("expected getter %q in Kind, got: %s", g, kind)
		}
	}
	// 5 getters: 4 shown + (+1) overflow
	if !strings.Contains(kind, "(+1)") {
		t.Errorf("expected (+1) overflow for 5 getters, got: %s", kind)
	}
}

func TestGenerate_Dart_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "app.dart")
	if ok {
		t.Error("expected fallthrough for empty Dart file")
	}
}

func TestGenerate_Dart_FallsThrough_NoStructure(t *testing.T) {
	src := `import 'dart:io';

void main() {
  print('Hello, World!');
}
`
	_, ok := filemap.Generate(src, "main.dart")
	if ok {
		t.Error("expected fallthrough for Dart script with no class/mixin/enum/extension")
	}
}
