package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_Dockerfile_MultiStage(t *testing.T) {
	src := `FROM node:18 AS deps
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production

FROM node:18 AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:18-slim AS runner
WORKDIR /app
COPY --from=builder /app/.next ./.next
EXPOSE 3000
CMD ["npm", "start"]
`
	fm, ok := filemap.Generate(src, "Dockerfile")
	if !ok {
		t.Fatal("expected deterministic map for Dockerfile")
	}

	if !strings.Contains(fm.Summary, "3") || !strings.Contains(fm.Summary, "stage") {
		t.Errorf("expected 3-stage summary, got: %s", fm.Summary)
	}

	if len(fm.Map) != 3 {
		t.Errorf("expected 3 map entries (one per stage), got %d: %v", len(fm.Map), fm.Map)
	}

	if len(fm.Symbols) != 3 {
		t.Errorf("expected 3 symbols (one per stage), got %d", len(fm.Symbols))
	}

	// First stage should name the image and alias.
	if !strings.Contains(fm.Map[0].Kind, "node:18") || !strings.Contains(fm.Map[0].Kind, "deps") {
		t.Errorf("first stage should mention image and alias, got: %s", fm.Map[0].Kind)
	}

	// Instruction verbs should appear in the stage label.
	if !strings.Contains(fm.Map[0].Kind, "WORKDIR") && !strings.Contains(fm.Map[0].Kind, "RUN") {
		t.Errorf("stage should list instruction verbs, got: %s", fm.Map[0].Kind)
	}
}

func TestGenerate_Dockerfile_SingleStage(t *testing.T) {
	src := `FROM ubuntu:22.04
RUN apt-get update && apt-get install -y curl
COPY . /app
WORKDIR /app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
`
	fm, ok := filemap.Generate(src, "Dockerfile")
	if !ok {
		t.Fatal("expected deterministic map for single-stage Dockerfile")
	}

	if !strings.Contains(fm.Summary, "1") || !strings.Contains(fm.Summary, "stage") {
		t.Errorf("expected 1-stage summary, got: %s", fm.Summary)
	}
	if len(fm.Map) != 1 {
		t.Errorf("expected 1 map entry, got %d", len(fm.Map))
	}
}

func TestGenerate_Dockerfile_LineRangesNoOverlap(t *testing.T) {
	src := `FROM alpine:3.18 AS build
RUN apk add --no-cache go
COPY . /src
RUN cd /src && go build -o app

FROM alpine:3.18
COPY --from=build /src/app /app
EXPOSE 9090
`
	fm, ok := filemap.Generate(src, "Dockerfile")
	if !ok {
		t.Fatal("expected deterministic map")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(fm.Map))
	}

	// First stage must end before second starts.
	for _, e := range fm.Map {
		if e.Lines == "" {
			t.Errorf("map entry has empty Lines: %+v", e)
		}
	}
}

func TestGenerate_Dockerfile_FallsThrough_NoFrom(t *testing.T) {
	src := "# just comments\nRUN echo hello\n"
	_, ok := filemap.Generate(src, "Dockerfile")
	if ok {
		t.Error("expected fallthrough for Dockerfile with no FROM instruction")
	}
}

func TestGenerate_Dockerfile_FallsThrough_Empty(t *testing.T) {
	_, ok := filemap.Generate("", "Dockerfile")
	if ok {
		t.Error("expected fallthrough for empty Dockerfile")
	}
}

func TestGenerate_Dockerfile_PlatformFlag(t *testing.T) {
	// FROM --platform=$BUILDPLATFORM should not capture the flag as the image.
	src := `FROM --platform=$BUILDPLATFORM golang:1.24 AS builder
RUN go build ./...

FROM --platform=linux/amd64 alpine:3.19
COPY --from=builder /out/app /app
`
	fm, ok := filemap.Generate(src, "Dockerfile")
	if !ok {
		t.Fatal("expected deterministic map for Dockerfile with --platform flags")
	}
	if len(fm.Map) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(fm.Map))
	}
	if strings.Contains(fm.Map[0].Kind, "--platform") {
		t.Errorf("--platform flag should not appear in stage label, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[0].Kind, "golang:1.24") {
		t.Errorf("first stage should name the image golang:1.24, got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[0].Kind, "builder") {
		t.Errorf("first stage should name the alias 'builder', got: %s", fm.Map[0].Kind)
	}
	if !strings.Contains(fm.Map[1].Kind, "alpine:3.19") {
		t.Errorf("second stage should name the image alpine:3.19, got: %s", fm.Map[1].Kind)
	}
}

func TestGenerate_Dockerfile_DotDockerfileExtension(t *testing.T) {
	src := "FROM golang:1.22 AS build\nRUN go build ./...\n"
	fm, ok := filemap.Generate(src, "backend.dockerfile")
	if !ok {
		t.Fatal("expected deterministic map for .dockerfile extension")
	}
	if !strings.Contains(fm.Map[0].Kind, "golang:1.22") {
		t.Errorf("stage should mention image, got: %s", fm.Map[0].Kind)
	}
}
