package filemap_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/filemap"
)

func TestGenerate_Rust_BasicModule(t *testing.T) {
	src := `//! HTTP client utilities for the API layer.
//! Provides connection pooling and retry logic.

use std::time::Duration;
use reqwest::Client;

pub struct HttpClient {
    client: Client,
    timeout: Duration,
}

impl HttpClient {
    pub fn new(timeout_secs: u64) -> Self {
        HttpClient {
            client: Client::new(),
            timeout: Duration::from_secs(timeout_secs),
        }
    }

    pub async fn get(&self, url: &str) -> Result<String, reqwest::Error> {
        self.client.get(url).send().await?.text().await
    }
}

pub fn default_client() -> HttpClient {
    HttpClient::new(30)
}

fn internal_helper(x: i32) -> i32 {
    x * 2
}
`
	fm, ok := filemap.Generate(src, "client.rs")
	if !ok {
		t.Fatal("expected deterministic map for .rs file")
	}

	if !strings.Contains(fm.Summary, "HTTP client") {
		t.Errorf("summary should come from //! doc comment, got: %s", fm.Summary)
	}

	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	// pub items should be symbols.
	for _, want := range []string{"HttpClient", "default_client"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q in %v", want, fm.Symbols)
		}
	}
	// private fn should not be in symbols (only pub items are tracked).
	if symbolSet["internal_helper"] {
		t.Errorf("private fn should not be in symbols: %v", fm.Symbols)
	}

	// use block should appear as first entry.
	if len(fm.Map) == 0 || fm.Map[0].Kind != "use declarations" {
		t.Errorf("expected first entry to be use declarations, got: %v", fm.Map)
	}

	// impl entry should be present.
	hasImpl := false
	for _, e := range fm.Map {
		if strings.Contains(e.Kind, "impl HttpClient") {
			hasImpl = true
		}
	}
	if !hasImpl {
		t.Errorf("expected impl HttpClient entry: %v", fm.Map)
	}
}

func TestGenerate_Rust_FallsThrough_NoStructure(t *testing.T) {
	_, ok := filemap.Generate(strings.Repeat("random text\n", 50), "lib.rs")
	if ok {
		t.Error("expected fallthrough for file with no Rust structure")
	}
}

func TestGenerate_Rust_EmptyFile(t *testing.T) {
	_, ok := filemap.Generate("", "main.rs")
	if ok {
		t.Error("expected fallthrough for empty Rust file")
	}
}

func TestGenerate_Rust_EnumAndTrait(t *testing.T) {
	src := `pub enum Status {
    Active,
    Inactive,
    Pending(String),
}

pub trait Describable {
    fn describe(&self) -> String;
    fn short_name(&self) -> &str;
}

impl Describable for Status {
    fn describe(&self) -> String {
        match self {
            Status::Active => "active".to_string(),
            Status::Inactive => "inactive".to_string(),
            Status::Pending(s) => format!("pending: {}", s),
        }
    }

    fn short_name(&self) -> &str {
        "Status"
    }
}
`
	fm, ok := filemap.Generate(src, "status.rs")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	symbolSet := map[string]bool{}
	for _, s := range fm.Symbols {
		symbolSet[s] = true
	}
	for _, want := range []string{"Status", "Describable"} {
		if !symbolSet[want] {
			t.Errorf("expected symbol %q, got: %v", want, fm.Symbols)
		}
	}
}

func TestGenerate_Rust_ModDecl(t *testing.T) {
	src := `pub mod config;
pub mod router;
mod internal;

use config::Config;

pub fn run(cfg: Config) {
    router::start(cfg);
}
`
	fm, ok := filemap.Generate(src, "lib.rs")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	hasRun := false
	for _, e := range fm.Map {
		if strings.Contains(e.Kind, "fn run") || strings.Contains(e.Kind, "pub fn run") {
			hasRun = true
		}
	}
	if !hasRun {
		t.Errorf("expected fn run entry: %v", fm.Map)
	}
}

func TestGenerate_Rust_LineRangesPresent(t *testing.T) {
	src := `use std::collections::HashMap;

pub struct Cache {
    store: HashMap<String, String>,
}

impl Cache {
    pub fn new() -> Self {
        Cache { store: HashMap::new() }
    }

    pub fn get(&self, key: &str) -> Option<&String> {
        self.store.get(key)
    }
}
`
	fm, ok := filemap.Generate(src, "cache.rs")
	if !ok {
		t.Fatal("expected deterministic map")
	}

	for _, e := range fm.Map {
		if e.Lines == "" {
			t.Errorf("map entry has empty Lines: %+v", e)
		}
	}
}
