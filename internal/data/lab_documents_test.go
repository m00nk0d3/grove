package data

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/m00nk0d3/grove/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteLabDocument_CreatesAtRepositoryPath(t *testing.T) {
	checkout := t.TempDir()
	target, err := WriteLabDocument(checkout, domain.LabArtifact{Path: "docs/adr/0003-cache.md", Body: "# Cache\n"})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(checkout, "docs", "adr", "0003-cache.md"), target)
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "# Cache\n", string(got))
}

func TestWriteLabDocument_ExtendsButNeverDrops(t *testing.T) {
	checkout := t.TempDir()
	path := filepath.Join(checkout, "CONTEXT.md")
	require.NoError(t, os.WriteFile(path, []byte("# Grove\r\n\r\n**Worktree**:\r\nA checkout.\r\n"), 0o644))

	extended := "# Grove\n\n**Worktree**:\nA checkout.\n\n**Lab entry**:\nAn idea or bug.\n"
	_, err := WriteLabDocument(checkout, domain.LabArtifact{Path: "CONTEXT.md", Body: extended})
	require.NoError(t, err, "a draft that keeps every line may replace the file")
	got, _ := os.ReadFile(path)
	assert.Equal(t, extended, string(got))

	_, err = WriteLabDocument(checkout, domain.LabArtifact{Path: "CONTEXT.md", Body: "# Grove\n\n**Lab entry**:\nAn idea or bug.\n"})
	assert.True(t, errors.Is(err, ErrLabDocumentWouldLoseContent))
	assert.ErrorContains(t, err, `"**Worktree**:"`, "the error names a line that would be lost")
	got, _ = os.ReadFile(path)
	assert.Equal(t, extended, string(got), "the checkout's file is untouched")
}

func TestLabDocumentTarget_StaysInsideTheCheckout(t *testing.T) {
	checkout := t.TempDir()
	for _, bad := range []string{"../outside.md", "docs/../../outside.md", ".git/config", ".GIT/hooks/pre-commit", "/etc/passwd", ""} {
		_, err := LabDocumentTarget(checkout, domain.LabArtifact{Path: bad})
		assert.Error(t, err, bad)
	}
	_, err := LabDocumentTarget(checkout, domain.LabArtifact{Path: "src/ordering/CONTEXT.md"})
	assert.NoError(t, err)
}

func TestLabDocumentChanges_ReportsAddedAndDroppedLines(t *testing.T) {
	checkout := t.TempDir()
	if err := os.WriteFile(filepath.Join(checkout, "CONTEXT.md"), []byte("# Shop\r\n\r\n**Order**: a request.\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	extended := domain.LabArtifact{Path: "CONTEXT.md", Body: "# Shop\n\n**Order**: a request.\n**Cart**: items before ordering.\n"}
	change, err := LabDocumentChanges(checkout, extended)
	if err != nil {
		t.Fatal(err)
	}
	if change.New || len(change.Added) != 1 || change.Added[0] != "**Cart**: items before ordering." || len(change.Dropped) != 0 {
		t.Errorf("change = %+v", change)
	}
	shrunk := domain.LabArtifact{Path: "CONTEXT.md", Body: "# Shop\n"}
	if change, _ := LabDocumentChanges(checkout, shrunk); len(change.Dropped) != 1 {
		t.Errorf("a draft that drops a line reports it: %+v", change)
	}
	fresh := domain.LabArtifact{Path: "docs/adr/0001-x.md", Body: "# Use SQLite\n\nBecause.\n"}
	if change, _ := LabDocumentChanges(checkout, fresh); !change.New || len(change.Added) != 2 {
		t.Errorf("a new document adds every line: %+v", change)
	}
}
