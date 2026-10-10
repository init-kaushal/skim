package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_YAML_DockerCompose(t *testing.T) {
	src := `version: "3.8"
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
    depends_on:
      - db
  db:
    image: postgres:15
    environment:
      POSTGRES_PASSWORD: secret
  redis:
    image: redis:7
volumes:
  db_data:
`
	fm, ok := filemap.Generate(src, "docker-compose.yml")
	if !ok {
		t.Fatal("expected deterministic map for docker-compose.yml")
	}

	if !strings.Contains(fm.Summary, "Docker Compose") {
		t.Errorf("expected Docker Compose summary, got: %s", fm.Summary)
	}

	// services and volumes should be top-level symbols.
	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	for _, want := range []string{"version", "services", "volumes"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q, got: %v", want, fm.Symbols)
		}
	}

	// services entry should mention its child services.
	for _, e := range fm.Map {
		if strings.HasPrefix(e.Kind, "services:") {
			if !strings.Contains(e.Kind, "web") && !strings.Contains(e.Kind, "db") {
				t.Errorf("services entry should list children, got: %s", e.Kind)
			}
			return
		}
	}
	t.Errorf("no services entry found: %v", fm.Map)
}

func TestGenerate_YAML_KubernetesDeployment(t *testing.T) {
	src := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: production
  labels:
    app: my-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
        - name: app
          image: my-app:latest
          ports:
            - containerPort: 8080
`
	fm, ok := filemap.Generate(src, "deployment.yaml")
	if !ok {
		t.Fatal("expected deterministic map for Kubernetes YAML")
	}

	if !strings.Contains(fm.Summary, "Kubernetes") {
		t.Errorf("expected Kubernetes summary, got: %s", fm.Summary)
	}

	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	for _, want := range []string{"apiVersion", "kind", "metadata", "spec"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q, got: %v", want, fm.Symbols)
		}
	}
}

func TestGenerate_YAML_GitHubActions(t *testing.T) {
	src := `name: CI
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - run: go test ./...
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: go vet ./...
`
	fm, ok := filemap.Generate(src, ".github/workflows/ci.yml")
	if !ok {
		t.Fatal("expected deterministic map for GitHub Actions")
	}

	if !strings.Contains(fm.Summary, "GitHub Actions") {
		t.Errorf("expected GitHub Actions summary, got: %s", fm.Summary)
	}
}

func TestGenerate_YAML_FallsThrough_NoStructure(t *testing.T) {
	_, ok := filemap.Generate(strings.Repeat("just some plain text\n", 30), "notes.yaml")
	if ok {
		t.Error("expected fallthrough for YAML file with no key-value structure")
	}
}

func TestGenerate_YAML_EmptyFile(t *testing.T) {
	_, ok := filemap.Generate("", "config.yml")
	if ok {
		t.Error("expected fallthrough for empty YAML file")
	}
}

func TestGenerate_YAML_LineRangesPresent(t *testing.T) {
	src := `name: my-service
port: 8080
database:
  host: localhost
  port: 5432
  name: mydb
`
	fm, ok := filemap.Generate(src, "config.yaml")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	for _, e := range fm.Map {
		if e.Lines == "" {
			t.Errorf("map entry has empty Lines: %+v", e)
		}
	}

	// name should be on line 1.
	for _, e := range fm.Map {
		if strings.HasPrefix(e.Kind, "name") && e.Lines != "1" {
			t.Errorf("name key: want line 1, got %s", e.Lines)
		}
	}
}
