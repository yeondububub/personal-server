package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"personal-server/internal/auth"
	"personal-server/internal/config"
	"personal-server/internal/db"
	"personal-server/internal/files"
	"personal-server/internal/media"
	"personal-server/internal/server"
	"personal-server/internal/share"
)

type dualHandlerTransport struct {
	pub  http.Handler
	priv http.Handler
}

func (rt dualHandlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	cloned := req.Clone(req.Context())
	cloned.RemoteAddr = "127.0.0.1:54321"
	cloned.RequestURI = req.URL.RequestURI()
	if req.Body == nil {
		cloned.Body = http.NoBody
	}
	if req.URL.Host == "priv.local" {
		rt.priv.ServeHTTP(rec, cloned)
	} else {
		rt.pub.ServeHTTP(rec, cloned)
	}
	res := rec.Result()
	res.Request = req
	return res, nil
}

func setupTestRouters(t *testing.T) (transport http.RoundTripper, mediaRoot string) {
	t.Helper()
	tmp := t.TempDir()
	realTmp, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatal(err)
	}

	mediaRoot = filepath.Join(realTmp, "media")
	uploadDir := filepath.Join(realTmp, "uploads")
	if err := os.MkdirAll(mediaRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		Server: config.ServerConfig{
			PublicAddr:    "127.0.0.1:8080",
			PrivateAddr:   "127.0.0.1:8081",
			CookieSecure:  false,
			PublicBaseURL: "https://test.duckdns.org",
		},
		Storage: config.StorageConfig{
			DatabasePath:  filepath.Join(realTmp, "test.db"),
			TempUploadDir: uploadDir,
			Roots: []config.MountRoot{
				{ID: "media", Name: "Media", Path: mediaRoot},
			},
		},
		Auth: config.AuthConfig{
			InitialAdminUsername: "admin",
			InitialAdminPassword: "testpassword123",
			SessionTTLHours:      24,
		},
	}

	conn, err := db.Open(cfg.Storage.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	authSvc, err := auth.NewService(conn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	jail := files.NewJail(cfg.Storage.Roots)
	filesH := files.NewHandler(jail, cfg.Storage.TempUploadDir)
	mediaH := media.NewHandler(conn, jail)
	shareH, err := share.NewHandler(conn, jail, cfg.Server.PublicBaseURL, false)
	if err != nil {
		t.Fatal(err)
	}

	mockWeb := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>DSM</title>")},
	}

	deps := server.Dependencies{
		Config:  cfg,
		Auth:    authSvc,
		Files:   filesH,
		Media:   mediaH,
		Share:   shareH,
		WebDist: mockWeb,
		Started: time.Now(),
	}

	return dualHandlerTransport{
		pub:  server.NewPublicRouter(deps),
		priv: server.NewPrivateRouter(deps),
	}, mediaRoot
}

func newCookieClient(t *testing.T, rt http.RoundTripper) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{
		Jar:       jar,
		Transport: rt,
	}
}

func TestDualListenersChunkedUploadRangeStreamAndShare(t *testing.T) {
	rt, mediaRoot := setupTestRouters(t)
	pubURL := "http://pub.local"
	privURL := "http://priv.local"

	adminClient := newCookieClient(t, rt)

	// 1. Login on Private listener
	loginBody := `{"username":"admin","password":"testpassword123"}`
	resp, err := adminClient.Post(privURL+"/api/auth/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 login, got %d", resp.StatusCode)
	}

	// Private capabilities should exist on Private listener
	privCapResp, err := adminClient.Get(privURL + "/api/private/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	_ = privCapResp.Body.Close()
	if privCapResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on private capabilities, got %d", privCapResp.StatusCode)
	}

	// 2. Login on Public listener & verify private route is NOT mounted (404)
	pubAdmin := newCookieClient(t, rt)
	pubLogin, err := pubAdmin.Post(pubURL+"/api/auth/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = pubLogin.Body.Close()

	pubCapResp, err := pubAdmin.Get(pubURL + "/api/private/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	_ = pubCapResp.Body.Close()
	if pubCapResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on public listener for private route, got %d", pubCapResp.StatusCode)
	}

	// 3. Chunked upload a video file + write a sibling SRT subtitle
	videoBytes := []byte("0123456789ABCDEF-MP4-PAYLOAD")
	initPayload := `{"rootId":"media","dirPath":"","fileName":"demo.mp4","totalSize":` + strconv.Itoa(len(videoBytes)) + `}`
	initResp, err := adminClient.Post(privURL+"/api/files/upload/init", "application/json", strings.NewReader(initPayload))
	if err != nil {
		t.Fatal(err)
	}
	var initData struct {
		UploadID string `json:"uploadId"`
	}
	if err := json.NewDecoder(initResp.Body).Decode(&initData); err != nil {
		t.Fatal(err)
	}
	_ = initResp.Body.Close()

	patchReq, err := http.NewRequest(http.MethodPatch, privURL+"/api/files/upload/"+initData.UploadID, bytes.NewReader(videoBytes))
	if err != nil {
		t.Fatal(err)
	}
	patchReq.Header.Set("X-Upload-Offset", "0")
	patchResp, err := adminClient.Do(patchReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("chunk patch failed: %d", patchResp.StatusCode)
	}

	compResp, err := adminClient.Post(privURL+"/api/files/upload/"+initData.UploadID+"/complete", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = compResp.Body.Close()
	if compResp.StatusCode != http.StatusOK {
		t.Fatalf("upload complete failed: %d", compResp.StatusCode)
	}

	if err := os.WriteFile(filepath.Join(mediaRoot, "demo.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\n자막 테스트\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Verify HTTP 206 Partial Content Range streaming
	rangeReq, _ := http.NewRequest(http.MethodGet, privURL+"/api/media/stream?root=media&path=demo.mp4", nil)
	rangeReq.Header.Set("Range", "bytes=0-9")
	rangeResp, err := adminClient.Do(rangeReq)
	if err != nil {
		t.Fatal(err)
	}
	partial, _ := io.ReadAll(rangeResp.Body)
	_ = rangeResp.Body.Close()
	if rangeResp.StatusCode != http.StatusPartialContent || string(partial) != "0123456789" {
		t.Fatalf("expected 206 '0123456789', got status=%d body=%q", rangeResp.StatusCode, string(partial))
	}

	// 5. Create password-protected public share link and access from anonymous client on Public listener
	shareCreateResp, err := adminClient.Post(
		privURL+"/api/shares",
		"application/json",
		strings.NewReader(`{"rootId":"media","relPath":"demo.mp4","password":"pw123","expiresHours":24,"allowDownload":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	var createdShare struct {
		Share struct {
			Token string `json:"token"`
		} `json:"share"`
	}
	if err := json.NewDecoder(shareCreateResp.Body).Decode(&createdShare); err != nil {
		t.Fatal(err)
	}
	_ = shareCreateResp.Body.Close()

	guest := newCookieClient(t, rt)
	lockedStream, err := guest.Get(pubURL + "/api/public/shares/" + createdShare.Share.Token + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	_ = lockedStream.Body.Close()
	if lockedStream.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 before unlock, got %d", lockedStream.StatusCode)
	}

	unlockResp, err := guest.Post(
		pubURL+"/api/public/shares/"+createdShare.Share.Token+"/unlock",
		"application/json",
		strings.NewReader(`{"password":"pw123"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = unlockResp.Body.Close()
	if unlockResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on share unlock, got %d", unlockResp.StatusCode)
	}

	unlockedSub, err := guest.Get(pubURL + "/api/public/shares/" + createdShare.Share.Token + "/subtitle?path=demo.srt")
	if err != nil {
		t.Fatal(err)
	}
	vttBytes, _ := io.ReadAll(unlockedSub.Body)
	_ = unlockedSub.Body.Close()
	if unlockedSub.StatusCode != http.StatusOK || !strings.Contains(string(vttBytes), "WEBVTT") {
		t.Fatalf("expected VTT subtitle on public share, status=%d body=%q", unlockedSub.StatusCode, string(vttBytes))
	}
}
