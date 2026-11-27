package files

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"personal-server/internal/config"
)

func TestHandleMoveSameAndCrossRoot(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rootA := filepath.Join(base, "rootA")
	rootB := filepath.Join(base, "rootB")
	rootRO := filepath.Join(base, "rootRO")
	if err := os.MkdirAll(filepath.Join(rootA, "sub", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rootB, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rootRO, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootA, "hello.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootA, "sub", "nested", "a.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	jail := NewJail([]config.MountRoot{
		{ID: "a", Name: "A", Path: rootA},
		{ID: "b", Name: "B", Path: rootB},
		{ID: "ro", Name: "RO", Path: rootRO, ReadOnly: true},
	})

	h := NewHandler(jail, filepath.Join(base, "uploads"))
	router := h.Routes("private")

	doMove := func(body map[string]string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/move", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := doMove(map[string]string{
		"fromRootId": "a",
		"fromRel":    "hello.txt",
		"toRootId":   "b",
		"toDirRel":   "",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("cross-root move failed: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(rootB, "hello.txt")); err != nil {
		t.Fatalf("expected file in rootB: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootA, "hello.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected source removed from rootA")
	}

	recSelf := doMove(map[string]string{
		"fromRootId": "a",
		"fromRel":    "sub",
		"toRootId":   "a",
		"toDirRel":   "sub/nested",
	})
	if recSelf.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when moving folder into descendant, got %d %s", recSelf.Code, recSelf.Body.String())
	}

	recRO := doMove(map[string]string{
		"fromRootId": "b",
		"fromRel":    "hello.txt",
		"toRootId":   "ro",
		"toDirRel":   "",
	})
	if recRO.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when moving into read-only root, got %d %s", recRO.Code, recRO.Body.String())
	}
}
