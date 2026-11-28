package settings

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"personal-server/internal/db"
)

func TestSettingsLifecycle(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	svc, err := NewService(conn, filepath.Join(dir, "branding"))
	if err != nil {
		t.Fatal(err)
	}

	pubRouter := svc.PublicRoutes()
	admRouter := svc.AdminRoutes()

	got, err := svc.GetPublic()
	if err != nil {
		t.Fatal(err)
	}
	if got.SiteTitle != "Personal Server" || got.ClockFormat != "24h" || got.WallpaperPreset != "ocean" {
		t.Fatalf("unexpected defaults: %+v", got)
	}

	updateBody := bytes.NewBufferString(`{"siteTitle":"My Home NAS","clockFormat":"12h","wallpaperPreset":"slate"}`)
	req := httptest.NewRequest(http.MethodPut, "/", updateBody)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	admRouter.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update failed: %d %s", rec.Code, rec.Body.String())
	}
	var updated PublicSettings
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.SiteTitle != "My Home NAS" || updated.ClockFormat != "12h" || updated.WallpaperPreset != "slate" {
		t.Fatalf("unexpected updated settings: %+v", updated)
	}

	var mpBuf bytes.Buffer
	mw := multipart.NewWriter(&mpBuf)
	fw, err := mw.CreateFormFile("file", "icon.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("\x89PNG\r\n\x1a\nfake"))
	_ = mw.Close()

	upReq := httptest.NewRequest(http.MethodPost, "/favicon", &mpBuf)
	upReq.Header.Set("Content-Type", mw.FormDataContentType())
	upRec := httptest.NewRecorder()
	admRouter.ServeHTTP(upRec, upReq)
	if upRec.Code != http.StatusOK {
		t.Fatalf("favicon upload failed: %d %s", upRec.Code, upRec.Body.String())
	}

	getFavReq := httptest.NewRequest(http.MethodGet, "/favicon", nil)
	getFavRec := httptest.NewRecorder()
	pubRouter.ServeHTTP(getFavRec, getFavReq)
	if getFavRec.Code != http.StatusOK || getFavRec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("expected 200 image/png, got %d %q", getFavRec.Code, getFavRec.Header().Get("Content-Type"))
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/favicon", nil)
	delRec := httptest.NewRecorder()
	admRouter.ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("favicon delete failed: %d %s", delRec.Code, delRec.Body.String())
	}

	getFavAfter := httptest.NewRecorder()
	pubRouter.ServeHTTP(getFavAfter, httptest.NewRequest(http.MethodGet, "/favicon", nil))
	if getFavAfter.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getFavAfter.Code)
	}
}
