package data

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/m00nk0d3/grove/internal/domain"
)

// ErrLabDocumentWouldLoseContent reports that an approved document would
// replace a file in the checkout without keeping all of its content.
var ErrLabDocumentWouldLoseContent = errors.New("the draft would drop content the checkout's file has")

// LabDocumentTarget returns where an approved repository document goes in the
// checkout: the same repository-relative path it was drafted at. A path that
// is absolute, climbs out of the checkout, or reaches into .git is refused.
func LabDocumentTarget(checkout string, a domain.LabArtifact) (string, error) {
	rel := filepath.FromSlash(a.Path)
	clean := filepath.Clean(rel)
	// A rooted path such as /etc/passwd is not absolute on Windows, which
	// wants a volume, but it is never repository-relative either.
	rooted := strings.HasPrefix(a.Path, "/") || strings.HasPrefix(a.Path, `\`)
	if rooted || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || clean == "." || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is not a path inside the repository", a.Path)
	}
	first := strings.SplitN(filepath.ToSlash(clean), "/", 2)[0]
	if strings.EqualFold(first, ".git") {
		return "", fmt.Errorf("%s is inside .git", a.Path)
	}
	return filepath.Join(checkout, clean), nil
}

// WriteLabDocument copies an approved repository document, such as CONTEXT.md
// or a decision record, into the checkout as an uncommitted change. An
// existing file is replaced only when the draft keeps every line of it, so an
// approval can extend the checkout's glossary but never drop from it; that is
// the case the grilling brief asks for, by starting the draft from the
// checkout's file. Nothing is committed.
func WriteLabDocument(checkout string, a domain.LabArtifact) (string, error) {
	target, err := LabDocumentTarget(checkout, a)
	if err != nil {
		return "", err
	}
	existing, err := os.ReadFile(target)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return "", fmt.Errorf("read %s: %w", a.Path, err)
	default:
		current := strings.ReplaceAll(string(existing), "\r\n", "\n")
		if current == a.Body {
			return target, nil
		}
		if missing := missingLines(current, a.Body); len(missing) > 0 {
			return "", fmt.Errorf("%s: %w, for example %q; edit the draft to keep it", a.Path, ErrLabDocumentWouldLoseContent, missing[0])
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("write %s: %w", a.Path, err)
	}
	if err := writeFileAtomic(target, []byte(a.Body)); err != nil {
		return "", err
	}
	return target, nil
}

// missingLines returns the non-blank lines of existing that draft lacks.
func missingLines(existing, draft string) []string {
	have := make(map[string]bool)
	for _, line := range strings.Split(draft, "\n") {
		have[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, line := range strings.Split(existing, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" && !have[trimmed] {
			missing = append(missing, trimmed)
		}
	}
	return missing
}
