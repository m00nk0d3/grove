package data

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoFixture writes files under a temporary checkout and returns it with the
// file list, as git ls-files would report it.
func repoFixture(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	var list []string
	for name, body := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		list = append(list, name)
	}
	return root, list
}

func TestBuildRepoMap_DescribesConventionsDomainAndLayout(t *testing.T) {
	root, files := repoFixture(t, map[string]string{
		"README.md":                       "# Shop\n\nA small shop.",
		"Makefile":                        "build:\n\tgo build\ntest: build\n\tgo test ./...\nVAR := 1\n",
		"CONTEXT.md":                      "# Shop\n\n**Order**: a request to buy items.\n",
		"docs/adr/0001-use-sqlite.md":     "# Use SQLite\n\n## Status\n\nAccepted.\n\n## Decision\n\nOrders are stored in SQLite. It is embedded.\n",
		"internal/orders/orders.go":       "// Package orders places and tracks orders. It owns the order lifecycle.\npackage orders\n",
		"internal/orders/orders_test.go":  "package orders\n",
		"web/package.json":                `{"description":"The storefront.","scripts":{"test":"vitest","build":"vite build"}}`,
		"web/src/cart.test.ts":            "",
		"web/node_modules/react/index.js": "",
		"web/node_modules/react/jsx.js":   "",
	})
	out := BuildRepoMap(root, files, "0123456789abcdef0123", 8000)

	for _, want := range []string{
		"Commit 0123456789ab",
		"- Languages: Go (2 files), TypeScript (1 file)",
		"Go tests beside the code (`*_test.go`), 1 file",
		"`Makefile` targets: build, test",
		"`web/package.json` scripts: build, test",
		"### CONTEXT.md\n\n# Shop\n\n**Order**: a request to buy items.",
		"`docs/adr/0001-use-sqlite.md` — Use SQLite: Orders are stored in SQLite.",
		"orders/ — places and tracks orders.",
		"web/ — The storefront.",
		"node_modules/ (2 files)",
		"cart.test.ts",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("map lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "jsx.js") {
		t.Error("vendored directories are collapsed")
	}
	if strings.Contains(out, "VAR") {
		t.Error("variable assignments are not make targets")
	}
}

func TestBuildRepoMap_FitsTheBudgetByTrimmingDeepLevels(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 400; i++ {
		files[fmt.Sprintf("pkg/area%d/sub/deep/file%d.go", i%20, i)] = "package deep\n"
	}
	root, list := repoFixture(t, files)
	out := BuildRepoMap(root, list, "abc", 300)
	if len(out) > 300*4 {
		t.Errorf("map is %d characters, over a 300-token budget", len(out))
	}
	if !strings.Contains(out, "area0/") || !strings.Contains(out, "(20 files)") {
		t.Errorf("trimmed directories keep their names and counts:\n%s", out)
	}
}

func TestLabStore_EnsureRepoMapBuildsOncePerCommit(t *testing.T) {
	s := newTestLabStore(t)
	root, files := repoFixture(t, map[string]string{"a.go": "package a\n"})
	path, err := s.EnsureRepoMap(root, files, "c1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := s.EnsureRepoMap(root, files, "c1", 0)
	if err != nil || again != path {
		t.Fatalf("again = %q, %v", again, err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "kept" {
		t.Error("an existing map for the commit is reused")
	}
	other, _ := s.EnsureRepoMap(root, files, "c2", 0)
	if raw, _ := os.ReadFile(other); !strings.Contains(string(raw), "# Repository map") {
		t.Error("a new commit gets its own map")
	}
}

// The Lab's evaluation harness gives its fixture repository this map, as
// Grove would build it. Run with UPDATE_GOLDEN=1 to regenerate it after
// changing the map.
func TestBuildRepoMap_EvalFixtureMatchesGolden(t *testing.T) {
	root := filepath.Join("..", "..", "runtime", "sandcastle", "evals", "fixtures", "repo")
	var files []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		files = append(files, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got := BuildRepoMap(root, files, "fixture", DefaultRepoMapTokens)
	golden := filepath.Join(root, "..", "repo-map.md")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read the golden map (run with UPDATE_GOLDEN=1 to create it): %v", err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
		t.Errorf("the fixture's map changed; run with UPDATE_GOLDEN=1 and review the difference")
	}
}
