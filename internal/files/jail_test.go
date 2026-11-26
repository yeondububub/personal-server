package files_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"personal-server/internal/config"
	"personal-server/internal/files"
)

func TestJailBlocksTraversalAndExternalSymlinks(t *testing.T) {
	base := t.TempDir()
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}

	mediaDir := filepath.Join(realBase, "media")
	outsideDir := filepath.Join(realBase, "outside")
	if err := os.MkdirAll(filepath.Join(mediaDir, "movies"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}

	secretFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("top-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	okFile := filepath.Join(mediaDir, "movies", "sample.mp4")
	if err := os.WriteFile(okFile, []byte("mp4data"), 0o644); err != nil {
		t.Fatal(err)
	}

	escapeLink := filepath.Join(mediaDir, "escape_link")
	if err := os.Symlink(secretFile, escapeLink); err != nil {
		t.Fatal(err)
	}

	jail := files.NewJail([]config.MountRoot{
		{ID: "media", Name: "Media", Path: mediaDir},
	})

	resolved, cleanRel, err := jail.ResolveExisting("media", "/movies/sample.mp4")
	if err != nil {
		t.Fatalf("expected valid path, got error: %v", err)
	}
	if cleanRel != "movies/sample.mp4" || resolved != okFile {
		t.Fatalf("unexpected resolved=(%s) cleanRel=(%s)", resolved, cleanRel)
	}

	if _, _, err := jail.ResolveExisting("media", "../outside/secret.txt"); !errors.Is(err, files.ErrPathTraversal) {
		t.Fatalf("expected ErrPathTraversal for .., got: %v", err)
	}

	if _, _, err := jail.ResolveExisting("media", "escape_link"); !errors.Is(err, files.ErrPathTraversal) {
		t.Fatalf("expected ErrPathTraversal for external symlink, got: %v", err)
	}

	if _, _, err := jail.ResolveCreateTarget("media", "escape_link"); !errors.Is(err, files.ErrPathTraversal) {
		t.Fatalf("expected ErrPathTraversal on overwriting external symlink, got: %v", err)
	}

	newTarget, newRel, err := jail.ResolveCreateTarget("media", "movies/notes.txt")
	if err != nil {
		t.Fatalf("expected valid create target, got: %v", err)
	}
	if err := files.AtomicWriteFile(newTarget, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("atomic write failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(mediaDir, filepath.FromSlash(newRel)))
	if err != nil || string(got) != "hello world" {
		t.Fatalf("unexpected file content: %q (%v)", string(got), err)
	}
}
