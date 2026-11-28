package settings

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"personal-server/internal/httputil"
)

const (
	defaultSiteTitle       = "Personal Server"
	defaultClockFormat     = "24h"
	defaultWallpaperMode   = "preset"
	defaultWallpaperPreset = "ocean"

	maxFaviconBytes   = 2 << 20
	maxWallpaperBytes = 15 << 20
)

const (
	keySiteTitle        = "site_title"
	keyClockFormat      = "clock_format"
	keyWallpaperMode    = "wallpaper_mode"
	keyWallpaperPreset  = "wallpaper_preset"
	keyWallpaperFile    = "wallpaper_file"
	keyFaviconFile      = "favicon_file"
)

type PublicSettings struct {
	SiteTitle          string `json:"siteTitle"`
	ClockFormat        string `json:"clockFormat"`
	WallpaperMode      string `json:"wallpaperMode"`
	WallpaperPreset    string `json:"wallpaperPreset"`
	HasCustomWallpaper bool   `json:"hasCustomWallpaper"`
	HasCustomFavicon   bool   `json:"hasCustomFavicon"`
	UpdatedAt          int64  `json:"updatedAt"`
}

type Service struct {
	db          *sql.DB
	brandingDir string

	mu sync.RWMutex
}

func NewService(conn *sql.DB, brandingDir string) (*Service, error) {
	if strings.TrimSpace(brandingDir) == "" {
		brandingDir = "./data/branding"
	}
	if err := os.MkdirAll(brandingDir, 0o755); err != nil {
		return nil, fmt.Errorf("create branding dir: %w", err)
	}
	return &Service{
		db:          conn,
		brandingDir: brandingDir,
	}, nil
}

func (s *Service) PublicRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", s.handleGetPublic)
	r.Get("/favicon", s.handleServeFavicon)
	r.Get("/wallpaper", s.handleServeWallpaper)
	return r
}

func (s *Service) AdminRoutes() chi.Router {
	r := chi.NewRouter()
	r.Put("/", s.handleUpdate)
	r.Post("/favicon", s.handleUploadFavicon)
	r.Delete("/favicon", s.handleDeleteFavicon)
	r.Post("/wallpaper", s.handleUploadWallpaper)
	r.Delete("/wallpaper", s.handleDeleteWallpaper)
	return r
}

func (s *Service) loadAllLocked() (map[string]string, int64, error) {
	rows, err := s.db.Query(`SELECT key, value, updated_at FROM app_settings`)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	kv := make(map[string]string)
	var latest int64
	for rows.Next() {
		var k, v string
		var ts int64
		if scanErr := rows.Scan(&k, &v, &ts); scanErr != nil {
			return nil, 0, scanErr
		}
		kv[k] = v
		if ts > latest {
			latest = ts
		}
	}
	return kv, latest, rows.Err()
}

func (s *Service) setKeyLocked(key, value string, now int64) error {
	_, err := s.db.Exec(
		`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, now,
	)
	return err
}

func (s *Service) fileExists(name string) bool {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return false
	}
	st, err := os.Stat(filepath.Join(s.brandingDir, name))
	return err == nil && !st.IsDir()
}

func (s *Service) GetPublic() (PublicSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	kv, latest, err := s.loadAllLocked()
	if err != nil {
		return PublicSettings{}, err
	}
	return s.buildPublic(kv, latest), nil
}

func (s *Service) buildPublic(kv map[string]string, latest int64) PublicSettings {
	siteTitle := strings.TrimSpace(kv[keySiteTitle])
	if siteTitle == "" {
		siteTitle = defaultSiteTitle
	}
	clockFormat := kv[keyClockFormat]
	if clockFormat != "12h" && clockFormat != "24h" {
		clockFormat = defaultClockFormat
	}
	preset := kv[keyWallpaperPreset]
	if preset != "ocean" && preset != "mist" && preset != "slate" {
		preset = defaultWallpaperPreset
	}
	hasWallpaper := s.fileExists(kv[keyWallpaperFile])
	hasFavicon := s.fileExists(kv[keyFaviconFile])

	mode := kv[keyWallpaperMode]
	if mode != "custom" || !hasWallpaper {
		mode = defaultWallpaperMode
	}

	return PublicSettings{
		SiteTitle:          siteTitle,
		ClockFormat:        clockFormat,
		WallpaperMode:      mode,
		WallpaperPreset:    preset,
		HasCustomWallpaper: hasWallpaper,
		HasCustomFavicon:   hasFavicon,
		UpdatedAt:          latest,
	}
}

func (s *Service) handleGetPublic(w http.ResponseWriter, _ *http.Request) {
	cfg, err := s.GetPublic()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "설정을 불러오지 못했습니다.")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, cfg)
}

func (s *Service) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SiteTitle       *string `json:"siteTitle"`
		ClockFormat     *string `json:"clockFormat"`
		WallpaperMode   *string `json:"wallpaperMode"`
		WallpaperPreset *string `json:"wallpaperPreset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청 형식입니다.")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	if req.SiteTitle != nil {
		title := strings.TrimSpace(*req.SiteTitle)
		if len([]rune(title)) > 60 {
			httputil.WriteError(w, http.StatusBadRequest, "탭 이름은 60자 이내로 입력하세요.")
			return
		}
		if title == "" {
			title = defaultSiteTitle
		}
		if err := s.setKeyLocked(keySiteTitle, title, now); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "설정 저장에 실패했습니다.")
			return
		}
	}
	if req.ClockFormat != nil {
		cf := strings.TrimSpace(*req.ClockFormat)
		if cf != "12h" && cf != "24h" {
			httputil.WriteError(w, http.StatusBadRequest, "시간 형식은 12h 또는 24h만 지원합니다.")
			return
		}
		if err := s.setKeyLocked(keyClockFormat, cf, now); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "설정 저장에 실패했습니다.")
			return
		}
	}
	if req.WallpaperPreset != nil {
		preset := strings.TrimSpace(*req.WallpaperPreset)
		if preset != "ocean" && preset != "mist" && preset != "slate" {
			httputil.WriteError(w, http.StatusBadRequest, "지원하지 않는 배경화면 프리셋입니다.")
			return
		}
		if err := s.setKeyLocked(keyWallpaperPreset, preset, now); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "설정 저장에 실패했습니다.")
			return
		}
	}
	if req.WallpaperMode != nil {
		mode := strings.TrimSpace(*req.WallpaperMode)
		if mode != "preset" && mode != "custom" {
			httputil.WriteError(w, http.StatusBadRequest, "배경화면 모드는 preset 또는 custom만 지원합니다.")
			return
		}
		if err := s.setKeyLocked(keyWallpaperMode, mode, now); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "설정 저장에 실패했습니다.")
			return
		}
	}

	kv, latest, err := s.loadAllLocked()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "설정을 불러오지 못했습니다.")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, s.buildPublic(kv, latest))
}

func validateImageExt(filename string, allowed map[string]string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(filename))
	_, ok := allowed[ext]
	return ext, ok
}

var (
	faviconAllowedExts = map[string]string{
		".ico":  "image/x-icon",
		".png":  "image/png",
		".svg":  "image/svg+xml",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".webp": "image/webp",
	}
	wallpaperAllowedExts = map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".webp": "image/webp",
	}
)

func (s *Service) saveUploadedAsset(r *http.Request, prefix string, maxBytes int64, allowed map[string]string) (string, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBytes+64*1024)
	if err := r.ParseMultipartForm(maxBytes + 64*1024); err != nil {
		return "", fmt.Errorf("파일 크기가 너무 크거나 잘못된 업로드 요청입니다")
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return "", fmt.Errorf("업로드할 파일이 없습니다")
	}
	defer func() { _ = file.Close() }()

	ext, ok := validateImageExt(header.Filename, allowed)
	if !ok {
		return "", fmt.Errorf("지원하지 않는 이미지 형식입니다")
	}

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("파일을 읽지 못했습니다")
	}
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("파일 최대 용량을 초과했습니다")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("빈 파일은 업로드할 수 없습니다")
	}

	targetName := prefix + ext
	targetPath := filepath.Join(s.brandingDir, targetName)
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return "", fmt.Errorf("파일 저장에 실패했습니다")
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("파일 저장에 실패했습니다")
	}
	return targetName, nil
}

func (s *Service) handleUploadFavicon(w http.ResponseWriter, r *http.Request) {
	newFile, err := s.saveUploadedAsset(r, "favicon", maxFaviconBytes, faviconAllowedExts)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	kv, _, err := s.loadAllLocked()
	if err == nil {
		oldFile := kv[keyFaviconFile]
		if oldFile != "" && oldFile != newFile && s.fileExists(oldFile) {
			_ = os.Remove(filepath.Join(s.brandingDir, oldFile))
		}
	}

	now := time.Now().Unix()
	if err := s.setKeyLocked(keyFaviconFile, newFile, now); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "파비콘 설정 저장 실패")
		return
	}
	kv, latest, err := s.loadAllLocked()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "설정을 불러오지 못했습니다.")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, s.buildPublic(kv, latest))
}

func (s *Service) handleDeleteFavicon(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	kv, _, err := s.loadAllLocked()
	if err == nil {
		if oldFile := kv[keyFaviconFile]; s.fileExists(oldFile) {
			_ = os.Remove(filepath.Join(s.brandingDir, oldFile))
		}
	}
	now := time.Now().Unix()
	if err := s.setKeyLocked(keyFaviconFile, "", now); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "파비콘 초기화 실패")
		return
	}
	kv, latest, err := s.loadAllLocked()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "설정을 불러오지 못했습니다.")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, s.buildPublic(kv, latest))
}

func (s *Service) handleUploadWallpaper(w http.ResponseWriter, r *http.Request) {
	newFile, err := s.saveUploadedAsset(r, "wallpaper", maxWallpaperBytes, wallpaperAllowedExts)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	kv, _, err := s.loadAllLocked()
	if err == nil {
		oldFile := kv[keyWallpaperFile]
		if oldFile != "" && oldFile != newFile && s.fileExists(oldFile) {
			_ = os.Remove(filepath.Join(s.brandingDir, oldFile))
		}
	}

	now := time.Now().Unix()
	if err := s.setKeyLocked(keyWallpaperFile, newFile, now); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "배경화면 저장 실패")
		return
	}
	if err := s.setKeyLocked(keyWallpaperMode, "custom", now); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "배경화면 모드 저장 실패")
		return
	}
	kv, latest, err := s.loadAllLocked()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "설정을 불러오지 못했습니다.")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, s.buildPublic(kv, latest))
}

func (s *Service) handleDeleteWallpaper(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	kv, _, err := s.loadAllLocked()
	if err == nil {
		if oldFile := kv[keyWallpaperFile]; s.fileExists(oldFile) {
			_ = os.Remove(filepath.Join(s.brandingDir, oldFile))
		}
	}
	now := time.Now().Unix()
	if err := s.setKeyLocked(keyWallpaperFile, "", now); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "배경화면 초기화 실패")
		return
	}
	if err := s.setKeyLocked(keyWallpaperMode, "preset", now); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "배경화면 모드 초기화 실패")
		return
	}
	kv, latest, err := s.loadAllLocked()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "설정을 불러오지 못했습니다.")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, s.buildPublic(kv, latest))
}

func (s *Service) serveAsset(w http.ResponseWriter, r *http.Request, key string, allowed map[string]string) {
	s.mu.RLock()
	kv, _, err := s.loadAllLocked()
	s.mu.RUnlock()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name := kv[key]
	if !s.fileExists(name) {
		http.NotFound(w, r)
		return
	}
	fullPath := filepath.Join(s.brandingDir, name)
	ext := strings.ToLower(filepath.Ext(name))
	if ct, ok := allowed[ext]; ok {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, fullPath)
}

func (s *Service) handleServeFavicon(w http.ResponseWriter, r *http.Request) {
	s.serveAsset(w, r, keyFaviconFile, faviconAllowedExts)
}

func (s *Service) handleServeWallpaper(w http.ResponseWriter, r *http.Request) {
	s.serveAsset(w, r, keyWallpaperFile, wallpaperAllowedExts)
}
