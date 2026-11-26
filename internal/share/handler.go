package share

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
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
	"personal-server/internal/media"
)

type ShareItem struct {
	Token         string `json:"token"`
	RootID        string `json:"rootId"`
	RelPath       string `json:"relPath"`
	Name          string `json:"name"`
	IsDir         bool   `json:"isDir"`
	HasPassword   bool   `json:"hasPassword"`
	AllowDownload bool   `json:"allowDownload"`
	ExpiresAt     int64  `json:"expiresAt"`
	CreatedAt     int64  `json:"createdAt"`
}

type Handler struct {
	db            *sql.DB
	jail          *files.Jail
	publicBaseURL string
	cookieSecure  bool
	hmacKey       []byte
}

func NewHandler(conn *sql.DB, jail *files.Jail, publicBaseURL string, cookieSecure bool) (*Handler, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	return &Handler{
		db:            conn,
		jail:          jail,
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
		cookieSecure:  cookieSecure,
		hmacKey:       secret,
	}, nil
}

func (h *Handler) AdminRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.handleListShares)
	r.Post("/", h.handleCreateShare)
	r.Delete("/{token}", h.handleDeleteShare)
	return r
}

func (h *Handler) PublicRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/{token}", h.handlePublicInfo)
	r.Post("/{token}/unlock", h.handlePublicUnlock)
	r.Get("/{token}/download", h.handlePublicDownload)
	r.Get("/{token}/stream", h.handlePublicStream)
	r.Get("/{token}/subtitle", h.handlePublicSubtitle)
	return r
}

func generateShareToken() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (h *Handler) signShareUnlock(token string, expiresAt int64) string {
	mac := hmac.New(sha256.New, h.hmacKey)
	mac.Write([]byte(token))
	var ts [8]byte
	for i := 0; i < 8; i++ {
		ts[i] = byte(expiresAt >> (8 * i))
	}
	mac.Write(ts[:])
	return hex.EncodeToString(mac.Sum(nil))
}

func shareCookieName(token string) string {
	return "ps_share_" + token
}

func (h *Handler) isShareUnlocked(r *http.Request, token string, pwHash string, expiresAt int64) bool {
	if pwHash == "" {
		return true
	}
	c, err := r.Cookie(shareCookieName(token))
	if err != nil || c.Value == "" {
		return false
	}
	expected := h.signShareUnlock(token, expiresAt)
	return hmac.Equal([]byte(c.Value), []byte(expected))
}

func (h *Handler) handleListShares(w http.ResponseWriter, _ *http.Request) {
	now := time.Now().Unix()
	_, _ = h.db.Exec(`DELETE FROM share_links WHERE expires_at <= ?`, now)

	rows, err := h.db.Query(
		`SELECT token, root_id, rel_path, is_dir, password_hash, allow_download, expires_at, created_at
		 FROM share_links
		 WHERE expires_at > ?
		 ORDER BY created_at DESC`,
		now,
	)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "공유 링크 목록을 불러오지 못했습니다.")
		return
	}
	defer func() { _ = rows.Close() }()

	items := make([]ShareItem, 0)
	for rows.Next() {
		var it ShareItem
		var isDirInt, allowDlInt int
		var pwHash string
		if err := rows.Scan(&it.Token, &it.RootID, &it.RelPath, &isDirInt, &pwHash, &allowDlInt, &it.ExpiresAt, &it.CreatedAt); err != nil {
			continue
		}
		it.IsDir = isDirInt == 1
		it.HasPassword = pwHash != ""
		it.AllowDownload = allowDlInt == 1
		it.Name = filepath.Base(it.RelPath)
		if it.RelPath == "" || it.Name == "." {
			it.Name = it.RootID
		}
		items = append(items, it)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"shares":        items,
		"publicBaseUrl": h.publicBaseURL,
	})
}

func (h *Handler) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RootID        string `json:"rootId"`
		RelPath       string `json:"relPath"`
		Password      string `json:"password"`
		ExpiresHours  int    `json:"expiresHours"`
		AllowDownload bool   `json:"allowDownload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청입니다.")
		return
	}
	if req.ExpiresHours <= 0 || req.ExpiresHours > 24*90 {
		req.ExpiresHours = 24
	}

	realPath, cleanRel, err := h.jail.ResolveExisting(req.RootID, req.RelPath)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	st, err := os.Stat(realPath)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "대상을 찾을 수 없습니다.")
		return
	}

	var pwHash string
	if strings.TrimSpace(req.Password) != "" {
		pwHash, err = auth.HashPassword(req.Password)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "비밀번호 해싱 실패")
			return
		}
	}

	token, err := generateShareToken()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "토큰 생성 실패")
		return
	}
	now := time.Now()
	expiresAt := now.Add(time.Duration(req.ExpiresHours) * time.Hour).Unix()
	isDirInt := 0
	if st.IsDir() {
		isDirInt = 1
	}
	allowDlInt := 0
	if req.AllowDownload {
		allowDlInt = 1
	}

	_, err = h.db.Exec(
		`INSERT INTO share_links (token, root_id, rel_path, is_dir, password_hash, allow_download, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		token, req.RootID, cleanRel, isDirInt, pwHash, allowDlInt, expiresAt, now.Unix(),
	)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "공유 링크 저장 실패")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{
		"share": ShareItem{
			Token:         token,
			RootID:        req.RootID,
			RelPath:       cleanRel,
			Name:          st.Name(),
			IsDir:         st.IsDir(),
			HasPassword:   pwHash != "",
			AllowDownload: req.AllowDownload,
			ExpiresAt:     expiresAt,
			CreatedAt:     now.Unix(),
		},
		"publicBaseUrl": h.publicBaseURL,
	})
}

func (h *Handler) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	_, _ = h.db.Exec(`DELETE FROM share_links WHERE token = ?`, token)
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type shareRecord struct {
	Token         string
	RootID        string
	RelPath       string
	IsDir         bool
	PasswordHash  string
	AllowDownload bool
	ExpiresAt     int64
}

func (h *Handler) lookupValidShare(token string) (shareRecord, error) {
	var rec shareRecord
	var isDirInt, allowDlInt int
	err := h.db.QueryRow(
		`SELECT token, root_id, rel_path, is_dir, password_hash, allow_download, expires_at
		 FROM share_links
		 WHERE token = ? AND expires_at > ?`,
		token, time.Now().Unix(),
	).Scan(&rec.Token, &rec.RootID, &rec.RelPath, &isDirInt, &rec.PasswordHash, &allowDlInt, &rec.ExpiresAt)
	if err != nil {
		return shareRecord{}, err
	}
	rec.IsDir = isDirInt == 1
	rec.AllowDownload = allowDlInt == 1
	return rec, nil
}

func (h *Handler) handlePublicInfo(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	rec, err := h.lookupValidShare(token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "만료되었거나 존재하지 않는 공유 링크입니다.")
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "공유 정보를 읽지 못했습니다.")
		return
	}

	unlocked := h.isShareUnlocked(r, rec.Token, rec.PasswordHash, rec.ExpiresAt)
	if !unlocked {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"token":            rec.Token,
			"locked":           true,
			"requiresPassword": true,
			"expiresAt":        rec.ExpiresAt,
		})
		return
	}

	realPath, cleanRel, err := h.jail.ResolveExisting(rec.RootID, rec.RelPath)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "원본 파일이 삭제되었거나 이동되었습니다.")
		return
	}
	st, err := os.Stat(realPath)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "원본 파일을 찾을 수 없습니다.")
		return
	}

	kind := files.ClassifyFile(st.Name(), st.IsDir())
	var subs []media.SubtitleTrack
	if kind == "video" {
		subs = media.FindSiblingSubtitles(realPath, cleanRel)
	}
	if subs == nil {
		subs = []media.SubtitleTrack{}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"token":            rec.Token,
		"locked":           false,
		"requiresPassword": rec.PasswordHash != "",
		"name":             st.Name(),
		"isDir":            st.IsDir(),
		"size":             st.Size(),
		"kind":             kind,
		"allowDownload":    rec.AllowDownload,
		"expiresAt":        rec.ExpiresAt,
		"subtitles":        subs,
	})
}

func (h *Handler) handlePublicUnlock(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	rec, err := h.lookupValidShare(token)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "만료되었거나 존재하지 않는 공유 링크입니다.")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청입니다.")
		return
	}
	if rec.PasswordHash != "" {
		ok, err := auth.VerifyPassword(req.Password, rec.PasswordHash)
		if err != nil || !ok {
			httputil.WriteError(w, http.StatusUnauthorized, "비밀번호가 일치하지 않습니다.")
			return
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     shareCookieName(rec.Token),
		Value:    h.signShareUnlock(rec.Token, rec.ExpiresAt),
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(rec.ExpiresAt, 0),
	})
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) handlePublicDownload(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	rec, err := h.lookupValidShare(token)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "만료되었거나 존재하지 않는 공유 링크입니다.")
		return
	}
	if !h.isShareUnlocked(r, rec.Token, rec.PasswordHash, rec.ExpiresAt) {
		httputil.WriteError(w, http.StatusForbidden, "비밀번호 인증이 필요합니다.")
		return
	}
	if !rec.AllowDownload {
		httputil.WriteError(w, http.StatusForbidden, "이 공유 링크는 다운로드가 비활성화되어 있습니다.")
		return
	}

	realPath, cleanRel, err := h.jail.ResolveExisting(rec.RootID, rec.RelPath)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "원본 파일을 찾을 수 없습니다.")
		return
	}
	st, err := os.Stat(realPath)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "원본 파일을 찾을 수 없습니다.")
		return
	}
	rootCfg, _ := h.jail.Root(rec.RootID)
	if st.IsDir() {
		zipName := st.Name() + ".zip"
		if cleanRel == "" {
			zipName = rec.RootID + ".zip"
		}
		files.StreamDirAsZip(w, realPath, rootCfg.Path, zipName)
		return
	}

	f, err := os.Open(realPath)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "파일을 열 수 없습니다.")
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(st.Name(), `"`, "_")+`"`)
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}

func (h *Handler) handlePublicStream(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	rec, err := h.lookupValidShare(token)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "만료되었거나 존재하지 않는 공유 링크입니다.")
		return
	}
	if !h.isShareUnlocked(r, rec.Token, rec.PasswordHash, rec.ExpiresAt) {
		httputil.WriteError(w, http.StatusForbidden, "비밀번호 인증이 필요합니다.")
		return
	}
	realPath, _, err := h.jail.ResolveExisting(rec.RootID, rec.RelPath)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "영상 파일을 찾을 수 없습니다.")
		return
	}
	media.ServeVideoRange(w, r, realPath)
}

func (h *Handler) handlePublicSubtitle(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	subRel := r.URL.Query().Get("path")
	rec, err := h.lookupValidShare(token)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "만료되었거나 존재하지 않는 공유 링크입니다.")
		return
	}
	if !h.isShareUnlocked(r, rec.Token, rec.PasswordHash, rec.ExpiresAt) {
		httputil.WriteError(w, http.StatusForbidden, "비밀번호 인증이 필요합니다.")
		return
	}

	videoReal, cleanRel, err := h.jail.ResolveExisting(rec.RootID, rec.RelPath)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "원본 파일을 찾을 수 없습니다.")
		return
	}
	allowed := false
	for _, tr := range media.FindSiblingSubtitles(videoReal, cleanRel) {
		if tr.Rel == subRel {
			allowed = true
			break
		}
	}
	if !allowed {
		httputil.WriteError(w, http.StatusForbidden, "허용되지 않은 자막 파일입니다.")
		return
	}
	subReal, _, err := h.jail.ResolveExisting(rec.RootID, subRel)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "자막 파일을 찾을 수 없습니다.")
		return
	}
	vtt, err := media.LoadAsWebVTT(subReal)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "자막 변환 실패")
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(vtt)
}
