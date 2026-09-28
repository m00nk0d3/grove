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
