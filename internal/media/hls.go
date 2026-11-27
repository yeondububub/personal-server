package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"personal-server/internal/httputil"
)

const (
	hlsIdleTimeout    = 60 * time.Second
	hlsReapInterval   = 15 * time.Second
	hlsPlaylistWait   = 12 * time.Second
	hlsPollInterval   = 200 * time.Millisecond
	vaapiFallbackSpan = 2 * time.Second
	vaapiDevicePath   = "/dev/dri/renderD128"
)

var (
	hlsSegmentPattern = regexp.MustCompile(`^seg_\d{4}\.ts$`)
	hexIDPattern      = regexp.MustCompile(`^[0-9a-f]{16,64}$`)
)

type ProbeResult struct {
	PlaybackMode    string  `json:"playbackMode"`
	VideoCodec      string  `json:"videoCodec"`
	AudioCodec      string  `json:"audioCodec"`
	FFmpegAvailable bool    `json:"ffmpegAvailable"`
	DurationSec     float64 `json:"durationSec"`
}

func DecidePlaybackMode(ext, videoCodec, audioCodec string, ffmpegAvailable bool) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	videoCodec = strings.ToLower(strings.TrimSpace(videoCodec))
	audioCodec = strings.ToLower(strings.TrimSpace(audioCodec))

	directExt := ext == ".mp4" || ext == ".m4v" || ext == ".webm"
	directVideo := videoCodec == "" || videoCodec == "h264" || videoCodec == "vp8" || videoCodec == "vp9" || videoCodec == "av1"
	directAudio := audioCodec == "" || audioCodec == "aac" || audioCodec == "mp3" || audioCodec == "opus" || audioCodec == "vorbis"

	if directExt && directVideo && directAudio {
		return "direct"
	}
	if ffmpegAvailable {
		return "hls"
	}
	return "direct"
}

func ProbeMedia(ctx context.Context, realPath string) ProbeResult {
	_, lookErr := exec.LookPath("ffmpeg")
	ffmpegAvailable := lookErr == nil
	var videoCodec, audioCodec string
	var durationSec float64

	if _, err := exec.LookPath("ffprobe"); err == nil {
		probeCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
		defer cancel()

		out, err := exec.CommandContext(
			probeCtx,
			"ffprobe",
			"-v", "quiet",
			"-print_format", "json",
			"-show_streams",
			"-show_format",
			realPath,
		).Output()
		if err == nil {
			var parsed struct {
				Streams []struct {
					CodecType string `json:"codec_type"`
					CodecName string `json:"codec_name"`
				} `json:"streams"`
				Format struct {
					Duration string `json:"duration"`
				} `json:"format"`
			}
			if json.Unmarshal(out, &parsed) == nil {
				for _, s := range parsed.Streams {
					switch strings.ToLower(s.CodecType) {
					case "video":
						if videoCodec == "" {
							videoCodec = strings.ToLower(strings.TrimSpace(s.CodecName))
						}
					case "audio":
						if audioCodec == "" {
							audioCodec = strings.ToLower(strings.TrimSpace(s.CodecName))
						}
					}
				}
				if rawDur := strings.TrimSpace(parsed.Format.Duration); rawDur != "" {
					if d, parseErr := strconv.ParseFloat(rawDur, 64); parseErr == nil && !math.IsNaN(d) && !math.IsInf(d, 0) && d > 0 {
						durationSec = d
					}
				}
			}
		}
	}

	return ProbeResult{
		PlaybackMode:    DecidePlaybackMode(filepath.Ext(realPath), videoCodec, audioCodec, ffmpegAvailable),
		VideoCodec:      videoCodec,
		AudioCodec:      audioCodec,
		FFmpegAvailable: ffmpegAvailable,
		DurationSec:     durationSec,
	}
}

func IsValidSessionID(id string) bool {
	return hexIDPattern.MatchString(id)
}

func IsValidHLSFile(name string) bool {
	return name == "index.m3u8" || hlsSegmentPattern.MatchString(name)
}

func VAAPIAvailable() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	f, err := os.Open(vaapiDevicePath)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func sanitizeStartSec(startSec float64) float64 {
	if math.IsNaN(startSec) || math.IsInf(startSec, 0) || startSec <= 0 {
		return 0
	}
	return startSec
}

func BuildFFmpegHLSArgs(inputPath, outDir string, useVAAPI bool, startSec float64) []string {
	segPattern := filepath.Join(outDir, "seg_%04d.ts")
	playlistPath := filepath.Join(outDir, "index.m3u8")
	cleanStart := sanitizeStartSec(startSec)

	if useVAAPI {
		args := []string{
			"-y",
			"-hwaccel", "vaapi",
			"-hwaccel_device", vaapiDevicePath,
			"-hwaccel_output_format", "vaapi",
		}
		if cleanStart > 0 {
			args = append(args, "-ss", fmt.Sprintf("%.3f", cleanStart))
		}
		return append(args,
			"-i", inputPath,
			"-vf", "format=nv12|vaapi,hwupload",
			"-c:v", "h264_vaapi",
			"-b:v", "6M",
			"-c:a", "aac",
			"-b:a", "192k",
			"-ac", "2",
			"-f", "hls",
			"-hls_time", "4",
			"-hls_list_size", "0",
			"-hls_flags", "independent_segments",
			"-hls_segment_filename", segPattern,
			playlistPath,
		)
	}

	args := []string{"-y"}
	if cleanStart > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", cleanStart))
	}
	return append(args,
		"-i", inputPath,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "192k",
		"-ac", "2",
		"-f", "hls",
		"-hls_time", "4",
		"-hls_list_size", "0",
		"-hls_flags", "independent_segments",
		"-hls_segment_filename", segPattern,
		playlistPath,
	)
}

type hlsSession struct {
	id        string
	dir       string
	inputPath string
	startSec  float64

	mu         sync.Mutex
	hwAccel    string
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	lastAccess time.Time
	stopped    bool
	exitErr    error

	done chan struct{}
}

func (s *hlsSession) touch() {
	s.mu.Lock()
	s.lastAccess = time.Now()
	s.mu.Unlock()
}

func (s *hlsSession) idleFor(now time.Time) time.Duration {
	s.mu.Lock()
	d := now.Sub(s.lastAccess)
	s.mu.Unlock()
	return d
}

func (s *hlsSession) stop() {
	s.mu.Lock()
	s.stopped = true
	cancel := s.cancel
	cmd := s.cmd
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	<-s.done
	_ = os.RemoveAll(s.dir)
}

func (s *hlsSession) err() error {
	s.mu.Lock()
	err := s.exitErr
	s.mu.Unlock()
	return err
}

type HLSManager struct {
	baseDir string

	mu       sync.Mutex
	sessions map[string]*hlsSession
}

func NewHLSManager(baseDir string) *HLSManager {
	_ = os.MkdirAll(baseDir, 0o755)
	return &HLSManager{
		baseDir:  baseDir,
		sessions: make(map[string]*hlsSession),
	}
}

func (m *HLSManager) StartBackground(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(hlsReapInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				m.shutdownAll()
				return
			case now := <-ticker.C:
				m.reapIdle(now)
			}
		}
	}()
}

func (m *HLSManager) reapIdle(now time.Time) {
	m.mu.Lock()
	var expired []*hlsSession
	for id, sess := range m.sessions {
		if sess.idleFor(now) > hlsIdleTimeout {
			delete(m.sessions, id)
			expired = append(expired, sess)
		}
	}
	m.mu.Unlock()

	for _, sess := range expired {
		sess.stop()
	}
}

func (m *HLSManager) shutdownAll() {
	m.mu.Lock()
	all := make([]*hlsSession, 0, len(m.sessions))
	for id, sess := range m.sessions {
		delete(m.sessions, id)
		all = append(all, sess)
	}
	m.mu.Unlock()

	for _, sess := range all {
		sess.stop()
	}

	entries, err := os.ReadDir(m.baseDir)
	if err == nil {
		for _, entry := range entries {
			_ = os.RemoveAll(filepath.Join(m.baseDir, entry.Name()))
		}
	}
}

func (m *HLSManager) StartSession(inputPath string, startSec float64) (string, string, float64, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", "", 0, errors.New("호스트에서 ffmpeg를 찾을 수 없습니다")
	}

	cleanStart := sanitizeStartSec(startSec)

	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", "", 0, fmt.Errorf("generate hls session id: %w", err)
	}
	id := hex.EncodeToString(raw)
	dir := filepath.Join(m.baseDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", 0, fmt.Errorf("create hls session dir: %w", err)
	}

	useVAAPI := VAAPIAvailable()
	hwAccel := "software"
	if useVAAPI {
		hwAccel = "vaapi"
	}

	procCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(procCtx, "ffmpeg", BuildFFmpegHLSArgs(inputPath, dir, useVAAPI, cleanStart)...)
	if err := cmd.Start(); err != nil {
		cancel()
		_ = os.RemoveAll(dir)
		return "", "", 0, fmt.Errorf("start ffmpeg: %w", err)
	}

	sess := &hlsSession{
		id:         id,
		dir:        dir,
		inputPath:  inputPath,
		startSec:   cleanStart,
		hwAccel:    hwAccel,
		cmd:        cmd,
		cancel:     cancel,
		lastAccess: time.Now(),
		done:       make(chan struct{}),
	}

	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	startedAt := time.Now()
	go m.superviseSession(sess, procCtx, cmd, useVAAPI, startedAt)

	return id, hwAccel, cleanStart, nil
}

func (m *HLSManager) superviseSession(sess *hlsSession, procCtx context.Context, cmd *exec.Cmd, usedVAAPI bool, startedAt time.Time) {
	defer close(sess.done)

	waitErr := cmd.Wait()
	if waitErr == nil {
		return
	}

	sess.mu.Lock()
	stopped := sess.stopped
	sess.mu.Unlock()
	if stopped {
		sess.mu.Lock()
		sess.exitErr = waitErr
		sess.mu.Unlock()
		return
	}

	playlistPath := filepath.Join(sess.dir, "index.m3u8")
	if usedVAAPI && time.Since(startedAt) <= vaapiFallbackSpan && !playlistHasSegments(playlistPath) {
		swCmd := exec.CommandContext(procCtx, "ffmpeg", BuildFFmpegHLSArgs(sess.inputPath, sess.dir, false, sess.startSec)...)
		if startErr := swCmd.Start(); startErr == nil {
			sess.mu.Lock()
			if sess.stopped {
				sess.mu.Unlock()
				_ = swCmd.Process.Kill()
				_ = swCmd.Wait()
				return
			}
			sess.cmd = swCmd
			sess.hwAccel = "software"
			sess.mu.Unlock()

			swErr := swCmd.Wait()
			sess.mu.Lock()
			sess.exitErr = swErr
			sess.mu.Unlock()
			return
		}
	}

	sess.mu.Lock()
	sess.exitErr = waitErr
	sess.mu.Unlock()
}

func (m *HLSManager) GetSession(id string) (*hlsSession, bool) {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	m.mu.Unlock()
	return sess, ok
}

func (m *HLSManager) StopSession(id string) {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	if ok {
		sess.stop()
	} else {
		_ = os.RemoveAll(filepath.Join(m.baseDir, id))
	}
}

func playlistHasSegments(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), ".ts")
}

func (h *Handler) handleHLSStart(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RootID   string  `json:"rootId"`
			RelPath  string  `json:"relPath"`
			StartSec float64 `json:"startSec"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청입니다.")
			return
		}
		realPath, _, err := h.jail.ResolveExistingForTier(req.RootID, req.RelPath, accessTier)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		st, err := os.Stat(realPath)
		if err != nil || st.IsDir() {
			httputil.WriteError(w, http.StatusBadRequest, "유효한 영상 파일이 아닙니다.")
			return
		}

		sessionID, hwAccel, startSec, err := h.hls.StartSession(realPath, req.StartSec)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, map[string]any{
			"sessionId":   sessionID,
			"playlistUrl": "/api/media/hls/" + sessionID + "/index.m3u8",
			"hwAccel":     hwAccel,
			"startSec":    startSec,
		})
	}
}

func (h *Handler) handleHLSFile(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	file := chi.URLParam(r, "file")
	if !IsValidSessionID(sessionID) || !IsValidHLSFile(file) {
		httputil.WriteError(w, http.StatusBadRequest, "잘못된 HLS 경로입니다.")
		return
	}

	sess, ok := h.hls.GetSession(sessionID)
	if !ok {
		httputil.WriteError(w, http.StatusNotFound, "HLS 세션을 찾을 수 없습니다.")
		return
	}
	sess.touch()

	targetPath := filepath.Join(sess.dir, file)
	if file == "index.m3u8" {
		deadline := time.NewTimer(hlsPlaylistWait)
		defer deadline.Stop()
		ticker := time.NewTicker(hlsPollInterval)
		defer ticker.Stop()

		for !playlistHasSegments(targetPath) {
			select {
			case <-r.Context().Done():
				return
			case <-sess.done:
				if !playlistHasSegments(targetPath) {
					msg := "HLS 트랜스코딩에 실패했습니다."
					if err := sess.err(); err != nil {
						msg = "HLS 트랜스코딩 오류: " + err.Error()
					}
					httputil.WriteError(w, http.StatusInternalServerError, msg)
					return
				}
			case <-deadline.C:
				httputil.WriteError(w, http.StatusGatewayTimeout, "HLS 플레이리스트 생성 시간이 초과되었습니다.")
				return
			case <-ticker.C:
			}
		}
	}

	f, err := os.Open(targetPath)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "HLS 세그먼트를 찾을 수 없습니다.")
		return
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil || st.IsDir() {
		httputil.WriteError(w, http.StatusNotFound, "HLS 세그먼트를 찾을 수 없습니다.")
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	if file == "index.m3u8" {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	http.ServeContent(w, r, file, st.ModTime(), f)
}

func (h *Handler) handleHLSDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	if !IsValidSessionID(sessionID) {
		httputil.WriteError(w, http.StatusBadRequest, "잘못된 세션 ID입니다.")
		return
	}
	h.hls.StopSession(sessionID)
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
