package server_test

import (
	"archive/zip"
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
	"personal-server/internal/docker"
	"personal-server/internal/files"
	"personal-server/internal/media"
	"personal-server/internal/monitor"
	"personal-server/internal/mounts"
	"personal-server/internal/server"
	"personal-server/internal/share"
	"personal-server/internal/terminal"
	"personal-server/internal/wargame"
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
			StacksDir:     filepath.Join(realTmp, "stacks"),
			Roots: []config.MountRoot{
				{ID: "media", Name: "Media", Path: mediaRoot},
			},
		},
		Auth: config.AuthConfig{
			InitialAdminUsername: "admin",
			InitialAdminPassword: "testpassword123",
			SessionTTLHours:      24,
		},
		Wargame: config.WargameConfig{
			BindIP:          "100.64.0.10",
			PortStart:       31000,
			PortEnd:         31099,
			WorkDir:         filepath.Join(realTmp, "wargames"),
			DefaultTTLHours: 3,
			MaxArchiveBytes: 64 * 1024 * 1024,
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
	mountSvc, err := mounts.NewService(conn, jail, cfg.Storage.Roots)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	filesH := files.NewHandler(jail, cfg.Storage.TempUploadDir)
	mediaH := media.NewHandler(conn, jail, filepath.Join(realTmp, "cache"))
	shareH, err := share.NewHandler(conn, jail, cfg.Server.PublicBaseURL, false)
	if err != nil {
		t.Fatal(err)
	}
	wgSvc, err := wargame.NewService(conn, jail, cfg.Wargame)
	if err != nil {
		t.Fatal(err)
	}
	monSvc := monitor.NewService(jail, started)
	termH := terminal.NewHandler(wgSvc)
	dockerSvc, err := docker.NewService(cfg.Storage.StacksDir)
	if err != nil {
		t.Fatal(err)
	}

	mockWeb := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>DSM</title>")},
	}

	deps := server.Dependencies{
		Config:   cfg,
		Auth:     authSvc,
		Jail:     jail,
		Files:    filesH,
		Media:    mediaH,
		Share:    shareH,
		Mounts:   mountSvc,
		Wargame:  wgSvc,
		Monitor:  monSvc,
		Terminal: termH,
		Docker:   dockerSvc,
		WebDist:  mockWeb,
		Started:  started,
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

func TestDynamicMountReadOnlyAndPrivateIsolation(t *testing.T) {
	rt, _ := setupTestRouters(t)
	pubURL := "http://pub.local"
	privURL := "http://priv.local"

	privClient := newCookieClient(t, rt)
	pubClient := newCookieClient(t, rt)
	loginBody := `{"username":"admin","password":"testpassword123"}`

	for _, pair := range []struct {
		c   *http.Client
		url string
	}{{privClient, privURL}, {pubClient, pubURL}} {
		r, err := pair.c.Post(pair.url+"/api/auth/login", "application/json", strings.NewReader(loginBody))
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("login failed on %s: %d", pair.url, r.StatusCode)
		}
	}

	// 1. Public listener must NOT expose /api/mounts or /api/mounts/browse-host
	pubMountsResp, err := pubClient.Get(pubURL + "/api/mounts")
	if err != nil {
		t.Fatal(err)
	}
	_ = pubMountsResp.Body.Close()
	if pubMountsResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for /api/mounts on public listener, got %d", pubMountsResp.StatusCode)
	}

	// 2. Create a local host directory with a real file and mount it dynamically via Private listener
	hostDir := t.TempDir()
	realHostDir, err := filepath.EvalSymlinks(hostDir)
	if err != nil {
		t.Fatal(err)
	}
	sampleFile := filepath.Join(realHostDir, "readme.txt")
	if err := os.WriteFile(sampleFile, []byte("local host file"), 0o644); err != nil {
		t.Fatal(err)
	}

	createPayload, _ := json.Marshal(map[string]any{
		"name":        "Secret Archive",
		"hostPath":    realHostDir,
		"readOnly":    true,
		"privateOnly": true,
	})
	createResp, err := privClient.Post(privURL+"/api/mounts", "application/json", bytes.NewReader(createPayload))
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		Mount struct {
			ID string `json:"id"`
		} `json:"mount"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	_ = createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated || created.Mount.ID == "" {
		t.Fatalf("expected 201 mount created, got %d", createResp.StatusCode)
	}

	// 3. Private listener can list & read files from the new mount
	listPriv, err := privClient.Get(privURL + "/api/files/list?root=" + created.Mount.ID + "&path=")
	if err != nil {
		t.Fatal(err)
	}
	listBody, _ := io.ReadAll(listPriv.Body)
	_ = listPriv.Body.Close()
	if listPriv.StatusCode != http.StatusOK || !strings.Contains(string(listBody), "readme.txt") {
		t.Fatalf("expected readme.txt in private listing, status=%d body=%s", listPriv.StatusCode, string(listBody))
	}

	// 4. Read-only mount rejects mkdir with 403
	mkdirResp, err := privClient.Post(
		privURL+"/api/files/mkdir",
		"application/json",
		strings.NewReader(`{"rootId":"`+created.Mount.ID+`","relPath":"blocked"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = mkdirResp.Body.Close()
	if mkdirResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 mkdir on read-only mount, got %d", mkdirResp.StatusCode)
	}

	// 5. Private-only mount is blocked on Public listener (403)
	listPub, err := pubClient.Get(pubURL + "/api/files/list?root=" + created.Mount.ID + "&path=")
	if err != nil {
		t.Fatal(err)
	}
	_ = listPub.Body.Close()
	if listPub.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for privateOnly mount on public listener, got %d", listPub.StatusCode)
	}

	// 6. Unmount removes it from API without deleting the actual file on disk
	delReq, _ := http.NewRequest(http.MethodDelete, privURL+"/api/mounts/"+created.Mount.ID, nil)
	delResp, err := privClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on unmount, got %d", delResp.StatusCode)
	}
	if _, err := os.Stat(sampleFile); err != nil {
		t.Fatalf("expected real file on disk to remain after unmount, got err: %v", err)
	}
}

func TestWargamePrivateOnlyPrepareDownloadAndDelete(t *testing.T) {
	rt, mediaRoot := setupTestRouters(t)
	pubURL := "http://pub.local"
	privURL := "http://priv.local"

	privClient := newCookieClient(t, rt)
	pubClient := newCookieClient(t, rt)
	loginBody := `{"username":"admin","password":"testpassword123"}`

	for _, pair := range []struct {
		c   *http.Client
		url string
	}{{privClient, privURL}, {pubClient, pubURL}} {
		r, err := pair.c.Post(pair.url+"/api/auth/login", "application/json", strings.NewReader(loginBody))
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("login failed on %s: %d", pair.url, r.StatusCode)
		}
	}

	pubWgResp, err := pubClient.Get(pubURL + "/api/wargames")
	if err != nil {
		t.Fatal(err)
	}
	_ = pubWgResp.Body.Close()
	if pubWgResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for /api/wargames on public listener, got %d", pubWgResp.StatusCode)
	}

	privWgResp, err := privClient.Get(privURL + "/api/wargames")
	if err != nil {
		t.Fatal(err)
	}
	var initialList struct {
		BindIP    string                    `json:"bindIp"`
		Instances []wargame.WargameInstance `json:"instances"`
	}
	if err := json.NewDecoder(privWgResp.Body).Decode(&initialList); err != nil {
		t.Fatal(err)
	}
	_ = privWgResp.Body.Close()
	if privWgResp.StatusCode != http.StatusOK || initialList.BindIP != "100.64.0.10" {
		t.Fatalf("expected 200 and bindIp 100.64.0.10 on private /api/wargames, got status=%d bindIp=%q", privWgResp.StatusCode, initialList.BindIP)
	}

	challDir := filepath.Join(mediaRoot, "bof101")
	if err := os.MkdirAll(challDir, 0o755); err != nil {
		t.Fatal(err)
	}
	elfPayload := append([]byte{0x7f, 'E', 'L', 'F'}, []byte("fake-elf-binary")...)
	if err := os.WriteFile(filepath.Join(challDir, "chall"), elfPayload, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(challDir, "readme.txt"), []byte("exploit me"), 0o644); err != nil {
		t.Fatal(err)
	}

	prepResp, err := privClient.Post(
		privURL+"/api/wargames/prepare/mount",
		"application/json",
		strings.NewReader(`{"rootId":"media","relPath":"bof101","name":"BOF 101"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	var prepared struct {
		Instance wargame.WargameInstance `json:"instance"`
	}
	if err := json.NewDecoder(prepResp.Body).Decode(&prepared); err != nil {
		t.Fatal(err)
	}
	_ = prepResp.Body.Close()
	if prepResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from prepare/mount, got %d", prepResp.StatusCode)
	}
	if prepared.Instance.Status != "detected" || prepared.Instance.Mode != "template" || prepared.Instance.TemplateKind != "pwn-socat" || prepared.Instance.EntrypointFile != "chall" {
		t.Fatalf("unexpected detected instance: %+v", prepared.Instance)
	}
	if len(prepared.Instance.Ports) != 1 || prepared.Instance.Ports[0].ContainerPort != 9000 || prepared.Instance.Ports[0].ConnKind != "nc" {
		t.Fatalf("unexpected detected ports: %+v", prepared.Instance.Ports)
	}

	extResp, err := privClient.Post(privURL+"/api/wargames/"+prepared.Instance.ID+"/extend", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	var extended struct {
		Instance wargame.WargameInstance `json:"instance"`
	}
	if err := json.NewDecoder(extResp.Body).Decode(&extended); err != nil {
		t.Fatal(err)
	}
	_ = extResp.Body.Close()
	if extResp.StatusCode != http.StatusOK || extended.Instance.ExpiresAt <= prepared.Instance.ExpiresAt {
		t.Fatalf("expected extended expiresAt > %d, got status=%d expiresAt=%d", prepared.Instance.ExpiresAt, extResp.StatusCode, extended.Instance.ExpiresAt)
	}

	srcResp, err := privClient.Get(privURL + "/api/wargames/" + prepared.Instance.ID + "/source")
	if err != nil {
		t.Fatal(err)
	}
	zipBytes, err := io.ReadAll(srcResp.Body)
	_ = srcResp.Body.Close()
	if err != nil || srcResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 source zip, got status=%d err=%v", srcResp.StatusCode, err)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("invalid source zip stream: %v", err)
	}
	foundNames := make(map[string]bool)
	for _, f := range zr.File {
		foundNames[f.Name] = true
	}
	if !foundNames["chall"] || !foundNames["readme.txt"] {
		t.Fatalf("expected chall and readme.txt in source zip, got %v", foundNames)
	}

	delReq, _ := http.NewRequest(http.MethodDelete, privURL+"/api/wargames/"+prepared.Instance.ID, nil)
	delResp, err := privClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on wargame delete, got %d", delResp.StatusCode)
	}

	afterListResp, err := privClient.Get(privURL + "/api/wargames")
	if err != nil {
		t.Fatal(err)
	}
	var afterList struct {
		Instances []wargame.WargameInstance `json:"instances"`
	}
	if err := json.NewDecoder(afterListResp.Body).Decode(&afterList); err != nil {
		t.Fatal(err)
	}
	_ = afterListResp.Body.Close()
	if len(afterList.Instances) != 0 {
		t.Fatalf("expected 0 instances after delete, got %d", len(afterList.Instances))
	}

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	zfWriter, err := zw.Create("webchall/app.py")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = zfWriter.Write([]byte("from flask import Flask\napp = Flask(__name__)\n"))
	_ = zw.Close()

	var mpBody bytes.Buffer
	boundary := "----psWargameTestBoundary"
	mpBody.WriteString("--" + boundary + "\r\n")
	mpBody.WriteString("Content-Disposition: form-data; name=\"name\"\r\n\r\nFlask Web\r\n")
	mpBody.WriteString("--" + boundary + "\r\n")
	mpBody.WriteString("Content-Disposition: form-data; name=\"file\"; filename=\"webchall.zip\"\r\n")
	mpBody.WriteString("Content-Type: application/zip\r\n\r\n")
	mpBody.Write(zipBuf.Bytes())
	mpBody.WriteString("\r\n--" + boundary + "--\r\n")

	uploadResp, err := privClient.Post(
		privURL+"/api/wargames/prepare/upload",
		"multipart/form-data; boundary="+boundary,
		&mpBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	var uploaded struct {
		Instance wargame.WargameInstance `json:"instance"`
	}
	if err := json.NewDecoder(uploadResp.Body).Decode(&uploaded); err != nil {
		t.Fatal(err)
	}
	_ = uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusCreated || uploaded.Instance.TemplateKind != "python-flask" || uploaded.Instance.Name != "Flask Web" {
		t.Fatalf("expected 201 python-flask from prepare/upload, got status=%d inst=%+v", uploadResp.StatusCode, uploaded.Instance)
	}
}

func TestPhase3MonitorAndTerminalIsolation(t *testing.T) {
	rt, _ := setupTestRouters(t)
	pubURL := "http://pub.local"
	privURL := "http://priv.local"

	privClient := newCookieClient(t, rt)
	pubClient := newCookieClient(t, rt)
	loginBody := `{"username":"admin","password":"testpassword123"}`

	for _, pair := range []struct {
		c   *http.Client
		url string
	}{{privClient, privURL}, {pubClient, pubURL}} {
		r, err := pair.c.Post(pair.url+"/api/auth/login", "application/json", strings.NewReader(loginBody))
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("login failed on %s: %d", pair.url, r.StatusCode)
		}
	}

	pubTermResp, err := pubClient.Get(pubURL + "/api/terminal/targets")
	if err != nil {
		t.Fatal(err)
	}
	_ = pubTermResp.Body.Close()
	if pubTermResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for /api/terminal/targets on public listener, got %d", pubTermResp.StatusCode)
	}

	privTermResp, err := privClient.Get(privURL + "/api/terminal/targets")
	if err != nil {
		t.Fatal(err)
	}
	var termData struct {
		Targets []terminal.Target `json:"targets"`
	}
	if err := json.NewDecoder(privTermResp.Body).Decode(&termData); err != nil {
		t.Fatal(err)
	}
	_ = privTermResp.Body.Close()
	if privTermResp.StatusCode != http.StatusOK || len(termData.Targets) == 0 || termData.Targets[0].Kind != "host" {
		t.Fatalf("expected 200 with host target on private /api/terminal/targets, got status=%d targets=%+v", privTermResp.StatusCode, termData.Targets)
	}

	for _, pair := range []struct {
		c    *http.Client
		url  string
		tier string
	}{{pubClient, pubURL, "public"}, {privClient, privURL, "private"}} {
		monResp, err := pair.c.Get(pair.url + "/api/monitor/snapshot")
		if err != nil {
			t.Fatal(err)
		}
		var snap monitor.MonitorSnapshot
		if err := json.NewDecoder(monResp.Body).Decode(&snap); err != nil {
			t.Fatal(err)
		}
		_ = monResp.Body.Close()
		if monResp.StatusCode != http.StatusOK || snap.AccessTier != pair.tier || snap.CPUCores <= 0 || snap.MemTotalBytes == 0 {
			t.Fatalf("unexpected /api/monitor/snapshot on %s: status=%d snap=%+v", pair.tier, monResp.StatusCode, snap)
		}
	}
}

func TestPhase4DockerIsolationAndStackCRUD(t *testing.T) {
	rt, _ := setupTestRouters(t)
	pubURL := "http://pub.local"
	privURL := "http://priv.local"

	privClient := newCookieClient(t, rt)
	pubClient := newCookieClient(t, rt)
	loginBody := `{"username":"admin","password":"testpassword123"}`

	for _, pair := range []struct {
		c   *http.Client
		url string
	}{{privClient, privURL}, {pubClient, pubURL}} {
		r, err := pair.c.Post(pair.url+"/api/auth/login", "application/json", strings.NewReader(loginBody))
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("login failed on %s: %d", pair.url, r.StatusCode)
		}
	}

	pubDockerResp, err := pubClient.Get(pubURL + "/api/docker/overview")
	if err != nil {
		t.Fatal(err)
	}
	_ = pubDockerResp.Body.Close()
	if pubDockerResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for /api/docker/overview on public listener, got %d", pubDockerResp.StatusCode)
	}

	privDockerResp, err := privClient.Get(privURL + "/api/docker/overview")
	if err != nil {
		t.Fatal(err)
	}
	var ov docker.Overview
	if err := json.NewDecoder(privDockerResp.Body).Decode(&ov); err != nil {
		t.Fatal(err)
	}
	_ = privDockerResp.Body.Close()
	if privDockerResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /api/docker/overview on private listener, got %d", privDockerResp.StatusCode)
	}

	capResp, err := privClient.Get(privURL + "/api/private/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	var caps struct {
		DockerEnabled bool `json:"dockerEnabled"`
		Phase         int  `json:"phase"`
	}
	if err := json.NewDecoder(capResp.Body).Decode(&caps); err != nil {
		t.Fatal(err)
	}
	_ = capResp.Body.Close()
	if !caps.DockerEnabled || caps.Phase != 4 {
		t.Fatalf("expected dockerEnabled=true phase=4, got %+v", caps)
	}

	composeYAML := "services:\n  web:\n    image: nginx:alpine\n"
	putBody, _ := json.Marshal(map[string]string{"composeYaml": composeYAML})
	putReq, _ := http.NewRequest(http.MethodPut, privURL+"/api/docker/stacks/demo-stack", bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := privClient.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on stack PUT, got %d", putResp.StatusCode)
	}

	getResp, err := privClient.Get(privURL + "/api/docker/stacks/demo-stack")
	if err != nil {
		t.Fatal(err)
	}
	var stackDoc struct {
		Name        string `json:"name"`
		ComposeYaml string `json:"composeYaml"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&stackDoc); err != nil {
		t.Fatal(err)
	}
	_ = getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK || stackDoc.Name != "demo-stack" || stackDoc.ComposeYaml != composeYAML {
		t.Fatalf("unexpected stack GET response: status=%d doc=%+v", getResp.StatusCode, stackDoc)
	}

	listResp, err := privClient.Get(privURL + "/api/docker/stacks")
	if err != nil {
		t.Fatal(err)
	}
	var stackList struct {
		Stacks []docker.StackSummary `json:"stacks"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&stackList); err != nil {
		t.Fatal(err)
	}
	_ = listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK || len(stackList.Stacks) != 1 || stackList.Stacks[0].Name != "demo-stack" {
		t.Fatalf("unexpected stack list: status=%d list=%+v", listResp.StatusCode, stackList.Stacks)
	}

	delReq, _ := http.NewRequest(http.MethodDelete, privURL+"/api/docker/stacks/demo-stack", nil)
	delResp, err := privClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on stack DELETE, got %d", delResp.StatusCode)
	}

	afterResp, err := privClient.Get(privURL + "/api/docker/stacks/demo-stack")
	if err != nil {
		t.Fatal(err)
	}
	_ = afterResp.Body.Close()
	if afterResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after stack DELETE, got %d", afterResp.StatusCode)
	}
}
