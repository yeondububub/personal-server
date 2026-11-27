package media

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"personal-server/internal/auth"
	"personal-server/internal/files"
	"personal-server/internal/httputil"
)

type Handler struct {
	db        *sql.DB
	jail      *files.Jail
	hls       *HLSManager
	thumbsDir string
}

func NewHandler(conn *sql.DB, jail *files.Jail, cacheDir string) *Handler {
	if strings.TrimSpace(cacheDir) == "" {
		cacheDir = "./data/cache"
	}
	hlsDir := filepath.Join(cacheDir, "hls")
	thumbsDir := filepath.Join(cacheDir, "thumbs")
	_ = os.MkdirAll(thumbsDir, 0o755)

	return &Handler{
		db:        conn,
		jail:      jail,
		hls:       NewHLSManager(hlsDir),
		thumbsDir: thumbsDir,
	}
}

func (h *Handler) StartBackground(ctx context.Context) {
	h.hls.StartBackground(ctx)
}

func (h *Handler) Routes(accessTier string) chi.Router {
	r := chi.NewRouter()
	r.Get("/info", h.handleInfo(accessTier))
	r.Get("/stream", h.handleStream(accessTier))
	r.Get("/subtitle", h.handleSubtitle(accessTier))
	r.Get("/thumbnail", h.handleThumbnail(accessTier))
	r.Put("/progress", h.handleSaveProgress(accessTier))
	r.Post("/hls/start", h.handleHLSStart(accessTier))
	r.Get("/hls/{sessionId}/{file}", h.handleHLSFile)
	r.Delete("/hls/{sessionId}", h.handleHLSDelete)
	return r
}

func VideoContentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".ogv":
		return "video/ogg"
	case ".mov":
		return "video/quicktime"
	case ".mkv":
		return "video/x-matroska"
	default:
		return ""
	}
}

func ServeVideoRange(w http.ResponseWriter, r *http.Request, realPath string) {
	st, err := os.Stat(realPath)
	if err != nil || st.IsDir() {
		httputil.WriteError(w, http.StatusNotFound, "영상 파일을 찾을 수 없습니다.")
		return
	}
	f, err := os.Open(realPath)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "영상 파일을 열 수 없습니다.")
		return
	}
	defer func() { _ = f.Close() }()

	if ct := VideoContentType(st.Name()); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

func (h *Handler) handleInfo(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.UserFromContext(r.Context())
		if !ok {
			httputil.WriteError(w, http.StatusUnauthorized, "로그인이 필요합니다.")
			return
		}
		rootID := r.URL.Query().Get("root")
		rel := r.URL.Query().Get("path")

		realPath, cleanRel, err := h.jail.ResolveExistingForTier(rootID, rel, accessTier)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		st, err := os.Stat(realPath)
		if err != nil || st.IsDir() {
			httputil.WriteError(w, http.StatusBadRequest, "유효한 영상 파일이 아닙니다.")
			return
		}

		var posSec, durSec float64
		err = h.db.QueryRow(
			`SELECT position_sec, duration_sec FROM media_progress WHERE user_id = ? AND root_id = ? AND rel_path = ?`,
			u.ID, rootID, cleanRel,
		).Scan(&posSec, &durSec)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			posSec = 0
			durSec = 0
		}

		tracks := FindSiblingSubtitles(realPath, cleanRel)
		if tracks == nil {
			tracks = []SubtitleTrack{}
		}

		probe := ProbeMedia(r.Context(), realPath)
		if probe.DurationSec > 0 {
			durSec = probe.DurationSec
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"name":            st.Name(),
			"rootId":          rootID,
			"relPath":         cleanRel,
			"size":            st.Size(),
			"positionSec":     posSec,
			"durationSec":     durSec,
			"subtitles":       tracks,
			"playbackMode":    probe.PlaybackMode,
			"videoCodec":      probe.VideoCodec,
			"audioCodec":      probe.AudioCodec,
			"ffmpegAvailable": probe.FFmpegAvailable,
		})
	}
}

func (h *Handler) handleStream(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rootID := r.URL.Query().Get("root")
		rel := r.URL.Query().Get("path")

		realPath, _, err := h.jail.ResolveExistingForTier(rootID, rel, accessTier)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		ServeVideoRange(w, r, realPath)
	}
}

func (h *Handler) handleSubtitle(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rootID := r.URL.Query().Get("root")
		rel := r.URL.Query().Get("path")

		realPath, _, err := h.jail.ResolveExistingForTier(rootID, rel, accessTier)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		vtt, err := LoadAsWebVTT(realPath)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "자막 변환에 실패했습니다.")
			return
		}
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(vtt)
	}
}

func (h *Handler) handleSaveProgress(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.UserFromContext(r.Context())
		if !ok {
			httputil.WriteError(w, http.StatusUnauthorized, "로그인이 필요합니다.")
			return
		}
		var req struct {
			RootID      string  `json:"rootId"`
			RelPath     string  `json:"relPath"`
			PositionSec float64 `json:"positionSec"`
			DurationSec float64 `json:"durationSec"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청입니다.")
			return
		}
		_, cleanRel, err := h.jail.ResolveExistingForTier(req.RootID, req.RelPath, accessTier)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.PositionSec < 0 {
			req.PositionSec = 0
		}
		_, err = h.db.Exec(
			`INSERT INTO media_progress (user_id, root_id, rel_path, position_sec, duration_sec, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(user_id, root_id, rel_path) DO UPDATE SET
			   position_sec = excluded.position_sec,
			   duration_sec = excluded.duration_sec,
			   updated_at = excluded.updated_at`,
			u.ID, req.RootID, cleanRel, req.PositionSec, req.DurationSec, time.Now().Unix(),
		)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "재생 위치 저장 실패")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
