package detect_test

import (
	"testing"

	"github.com/kaushal/skim/internal/config"
	"github.com/kaushal/skim/internal/detect"
)

// TestDefaultPassthroughGlobs_Credentials exercises the actual glob-matching
// path used by the hook for credential and lock-file paths from
// config.Default(). This is the critical test: it verifies that the glob
// patterns in configuration actually block the paths they're supposed to
// protect, at both root level and nested paths.
func TestDefaultPassthroughGlobs_Credentials(t *testing.T) {
	globs := config.Default().PassthroughGlobs

	cases := []struct {
		path  string
		match bool
		desc  string
	}{
		// .env files — root and nested
		{".env", true, ".env at root"},
		{"config/.env", true, ".env nested"},
		{"src/backend/.env", true, ".env deeply nested"},
		{".env.production", true, ".env.production at root"},
		{"deploy/.env.staging", true, ".env.staging nested"},
		{".env.local", true, ".env.local"},

		// Private key files — common names and nested
		{"id_rsa", true, "id_rsa at root"},
		{"~/.ssh/id_rsa", true, "id_rsa nested"},
		{"id_ed25519", true, "id_ed25519 at root"},
		{"secrets/id_ed25519", true, "id_ed25519 nested"},
		{"id_ecdsa", true, "id_ecdsa"},
		{"id_dsa", true, "id_dsa"},

		// Certificate and key extensions
		{"server.pem", true, ".pem at root"},
		{"certs/server.pem", true, ".pem nested"},
		{"certs/ca.key", true, ".key nested"},
		{"tls/client.p12", true, ".p12"},
		{"tls/keystore.pfx", true, ".pfx"},

		// Lock files — globs protected from maps
		{"yarn.lock", true, "yarn.lock at root"},
		{"frontend/yarn.lock", true, "yarn.lock nested"},
		{"go.sum", true, "go.sum at root"},
		{"module/go.sum", true, "go.sum nested"},
		{"pnpm-lock.yaml", true, "pnpm-lock.yaml"},
		{"Pipfile.lock", true, "Pipfile.lock"},
		{"Cargo.lock", true, "Cargo.lock"},

		// Files that must NOT be caught (false negatives would be over-blocking)
		{"main.go", false, "regular Go source"},
		{"README.md", true, ".md is already protected"},
		{"config.yaml", false, "regular YAML config"},
		{"environment.ts", false, ".ts file with 'env' in name"},
		{"myenv.sh", false, "file with env in name but not .env pattern"},
		{"keys.json", false, "json file with 'keys' in name"},
	}

	for _, tc := range cases {
		got := detect.MatchPassthrough(tc.path, globs)
		if got != tc.match {
			t.Errorf("MatchPassthrough(%q) [%s] = %v, want %v", tc.path, tc.desc, got, tc.match)
		}
	}
}
