package mounts

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"personal-server/internal/config"
	"personal-server/internal/files"
	"personal-server/internal/httputil"
)

type MountInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	HostPath    string `json:"hostPath"`
	ReadOnly    bool   `json:"readOnly"`
	PrivateOnly bool   `json:"privateOnly"`
	Exists      bool   `json:"exists"`
	CreatedAt   int64  `json:"createdAt"`
}

type Service struct {
	db   *sql.DB
	jail *files.Jail
}

func NewService(conn *sql.DB, jail *files.Jail, seedRoots []config.MountRoot) (*Service, error) {
	svc := &Service{
		db:   conn,
		jail: jail,
	}
	if err := svc.seedIfEmpty(seedRoots); err != nil {
		return nil, err
	}
	if err := svc.SyncJail(); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *Service) seedIfEmpty(seedRoots []config.MountRoot) error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM mount_roots`).Scan(&count); err != nil {
		return fmt.Errorf("count mount_roots: %w", err)
	}
	if count > 0 {
		return nil
	}
	now := time.Now().Unix()
	for idx, r := range seedRoots {
		roInt := 0
		if r.ReadOnly {
			roInt = 1
		}
		privInt := 0
		if r.PrivateOnly {
			privInt = 1
		}
		if _, err := s.db.Exec(
			`INSERT INTO mount_roots (id, name, host_path, read_only, private_only, sort_order, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			r.ID, r.Name, r.Path, roInt, privInt, idx, now,
		); err != nil {
			return fmt.Errorf("seed mount_root %s: %w", r.ID, err)
		}
	}
	return nil
}

func (s *Service) SyncJail() error {
	items, err := s.ListAll()
	if err != nil {
		return err
	}
	roots := make([]config.MountRoot, 0, len(items))
	for _, it := range items {
		if !it.Exists {
			continue
		}
		roots = append(roots, config.MountRoot{
			ID:          it.ID,
			Name:        it.Name,
			Path:        it.HostPath,
			ReadOnly:    it.ReadOnly,
			PrivateOnly: it.PrivateOnly,
		})
	}
	s.jail.Reload(roots)
	return nil
}

func (s *Service) ListAll() ([]MountInfo, error) {
	rows, err := s.db.Query(
		`SELECT id, name, host_path, read_only, private_only, created_at
		 FROM mount_roots
		 ORDER BY sort_order ASC, created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]MountInfo, 0)
	for rows.Next() {
		var it MountInfo
		var roInt, privInt int
		if err := rows.Scan(&it.ID, &it.Name, &it.HostPath, &roInt, &privInt, &it.CreatedAt); err != nil {
			continue
		}
		it.ReadOnly = roInt == 1
		it.PrivateOnly = privInt == 1
		if st, statErr := os.Stat(it.HostPath); statErr == nil && st.IsDir() {
			it.Exists = true
		}
		items = append(items, it)
	}
	return items, nil
}

var blockedVirtualPrefixes = []string{"/proc", "/sys", "/dev"}

func validateHostDir(rawPath string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", errors.New("호스트 디렉토리 경로를 입력하세요.")
	}
	if strings.HasPrefix(trimmed, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			if trimmed == "~" {
				trimmed = home
			} else if strings.HasPrefix(trimmed, "~/") {
				trimmed = filepath.Join(home, trimmed[2:])
			}
		}
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("유효하지 않은 경로입니다: %w", err)
	}
	realPath, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("존재하지 않는 디렉토리입니다.")
		}
		return "", fmt.Errorf("경로에 접근할 수 없습니다: %w", err)
	}

	for _, blocked := range blockedVirtualPrefixes {
		if realPath == blocked || strings.HasPrefix(realPath, blocked+"/") {
			return "", fmt.Errorf("커널 가상 파일 시스템(%s)은 마운트할 수 없습니다.", blocked)
		}
	}

	st, err := os.Stat(realPath)
	if err != nil {
		return "", fmt.Errorf("디렉토리 정보를 읽을 수 없습니다: %w", err)
	}
	if !st.IsDir() {
		return "", errors.New("파일이 아닌 디렉토리 경로를 지정해야 합니다.")
	}
	if _, err := os.ReadDir(realPath); err != nil {
		return "", fmt.Errorf("서버 프로세스에 해당 디렉토리 읽기 권한이 없습니다: %w", err)
	}
	return realPath, nil
}

var slugNonAlnumRe = regexp.MustCompile(`[^a-z0-9-]+`)

func slugifyID(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	slug := slugNonAlnumRe.ReplaceAllString(lower, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 32 {
		slug = slug[:32]
	}
	if slug == "" {
		slug = "mount"
	}
	return slug
}

func (s *Service) ensureUniqueID(preferred string) string {
	base := slugifyID(preferred)
	candidate := base
	for i := 0; i < 10; i++ {
		var exists int
		err := s.db.QueryRow(`SELECT COUNT(*) FROM mount_roots WHERE id = ?`, candidate).Scan(&exists)
		if err == nil && exists == 0 {
			return candidate
		}
		buf := make([]byte, 2)
		_, _ = rand.Read(buf)
		candidate = base + "-" + hex.EncodeToString(buf)
	}
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return "mount-" + hex.EncodeToString(buf)
}

func (s *Service) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", s.handleList)
	r.Post("/", s.handleCreate)
	r.Put("/{id}", s.handleUpdate)
	r.Delete("/{id}", s.handleDelete)
	r.Get("/browse-host", s.handleBrowseHost)
	return r
}

func (s *Service) handleList(w http.ResponseWriter, _ *http.Request) {
	items, err := s.ListAll()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "마운트 목록을 불러오지 못했습니다.")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"mounts": items})
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		HostPath    string `json:"hostPath"`
		ReadOnly    bool   `json:"readOnly"`
		PrivateOnly bool   `json:"privateOnly"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청 형식입니다.")
		return
	}

	realPath, err := validateHostDir(req.HostPath)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		base := filepath.Base(realPath)
		if base == "/" || base == "." {
			name = "Root"
		} else {
			name = base
		}
	}

	idSeed := strings.TrimSpace(req.ID)
	if idSeed == "" {
		idSeed = filepath.Base(realPath)
	}
	id := s.ensureUniqueID(idSeed)

	var maxOrder int
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(sort_order), -1) FROM mount_roots`).Scan(&maxOrder)

	roInt := 0
	if req.ReadOnly {
		roInt = 1
	}
	privInt := 0
	if req.PrivateOnly {
		privInt = 1
	}
	now := time.Now().Unix()

	if _, err := s.db.Exec(
		`INSERT INTO mount_roots (id, name, host_path, read_only, private_only, sort_order, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, name, realPath, roInt, privInt, maxOrder+1, now,
	); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "마운트 저장에 실패했습니다.")
		return
	}
	if err := s.SyncJail(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "마운트 반영에 실패했습니다.")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{
		"mount": MountInfo{
			ID:          id,
			Name:        name,
			HostPath:    realPath,
			ReadOnly:    req.ReadOnly,
			PrivateOnly: req.PrivateOnly,
			Exists:      true,
			CreatedAt:   now,
		},
	})
}

func (s *Service) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name        string `json:"name"`
		ReadOnly    bool   `json:"readOnly"`
		PrivateOnly bool   `json:"privateOnly"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청 형식입니다.")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "표시 이름을 입력하세요.")
		return
	}

	roInt := 0
	if req.ReadOnly {
		roInt = 1
	}
	privInt := 0
	if req.PrivateOnly {
		privInt = 1
	}
	res, err := s.db.Exec(
		`UPDATE mount_roots SET name = ?, read_only = ?, private_only = ? WHERE id = ?`,
		name, roInt, privInt, id,
	)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "마운트 수정에 실패했습니다.")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		httputil.WriteError(w, http.StatusNotFound, "해당 마운트를 찾을 수 없습니다.")
		return
	}
	if err := s.SyncJail(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "마운트 반영에 실패했습니다.")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := s.db.Exec(`DELETE FROM mount_roots WHERE id = ?`, id)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "마운트 해제에 실패했습니다.")
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		httputil.WriteError(w, http.StatusNotFound, "해당 마운트를 찾을 수 없습니다.")
		return
	}
	if err := s.SyncJail(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "마운트 반영에 실패했습니다.")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type quickPath struct {
	Label string `json:"label"`
	Path  string `json:"path"`
}

func discoverQuickPaths() []quickPath {
	candidates := make([]quickPath, 0, 8)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, quickPath{Label: "홈 (~)", Path: home})
	}
	candidates = append(candidates, quickPath{Label: "루트 (/)", Path: "/"})
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			quickPath{Label: "/Users", Path: "/Users"},
			quickPath{Label: "/Volumes", Path: "/Volumes"},
		)
	} else {
		candidates = append(candidates,
			quickPath{Label: "/home", Path: "/home"},
			quickPath{Label: "/mnt", Path: "/mnt"},
			quickPath{Label: "/media", Path: "/media"},
			quickPath{Label: "/data", Path: "/data"},
		)
	}

	out := make([]quickPath, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, c := range candidates {
		if _, dup := seen[c.Path]; dup {
			continue
		}
		if st, err := os.Stat(c.Path); err == nil && st.IsDir() {
			seen[c.Path] = struct{}{}
			out = append(out, c)
		}
	}
	return out
}

func (s *Service) handleBrowseHost(w http.ResponseWriter, r *http.Request) {
	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	showHidden := r.URL.Query().Get("hidden") == "1" || r.URL.Query().Get("hidden") == "true"

	if rawPath == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			rawPath = home
		} else {
			rawPath = "/"
		}
	}
	if strings.HasPrefix(rawPath, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			if rawPath == "~" {
				rawPath = home
			} else if strings.HasPrefix(rawPath, "~/") {
				rawPath = filepath.Join(home, rawPath[2:])
			}
		}
	}

	abs, err := filepath.Abs(rawPath)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "유효하지 않은 경로입니다.")
		return
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "디렉토리를 읽을 수 없습니다: "+err.Error())
		return
	}

	type dirEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	dirs := make([]dirEntry, 0)
	for _, de := range entries {
		name := de.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(abs, name)
		blocked := false
		for _, prefix := range blockedVirtualPrefixes {
			if full == prefix || strings.HasPrefix(full, prefix+"/") {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		isDir := de.IsDir()
		if !isDir && de.Type()&os.ModeSymlink != 0 {
			if st, statErr := os.Stat(full); statErr == nil && st.IsDir() {
				isDir = true
			}
		}
		if !isDir {
			continue
		}
		dirs = append(dirs, dirEntry{
			Name: name,
			Path: full,
		})
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	parent := filepath.Dir(abs)
	if parent == abs {
		parent = ""
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"currentPath": abs,
		"parentPath":  parent,
		"quickPaths":  discoverQuickPaths(),
		"directories": dirs,
	})
}
