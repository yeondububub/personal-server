package files

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"personal-server/internal/httputil"
)

const (
	maxEditableSize = 5 * 1024 * 1024 // 5 MB
	defaultChunkSz  = 8 * 1024 * 1024 // 8 MB
)

type Entry struct {
	Name    string `json:"name"`
	RelPath string `json:"relPath"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"`
	Kind    string `json:"kind"`
}

type Handler struct {
	jail          *Jail
	tempUploadDir string
}

func NewHandler(jail *Jail, tempUploadDir string) *Handler {
	return &Handler{
		jail:          jail,
		tempUploadDir: tempUploadDir,
	}
}

func ClassifyFile(name string, isDir bool) string {
	if isDir {
		return "dir"
	}
	lower := strings.ToLower(name)
	switch lower {
	case "dockerfile", "makefile", "caddyfile", "vagrantfile", ".gitignore", ".dockerignore", ".env":
		return "code"
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".mp4", ".webm", ".m4v", ".mov", ".mkv", ".avi", ".ogv":
		return "video"
	case ".mp3", ".wav", ".flac", ".m4a", ".ogg", ".aac":
		return "audio"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp":
		return "image"
	case ".zip", ".tar", ".gz", ".tgz", ".xz", ".7z", ".rar":
		return "archive"
	case ".txt", ".md", ".json", ".yaml", ".yml", ".toml", ".ini", ".conf", ".cfg",
		".sh", ".bash", ".zsh", ".py", ".js", ".ts", ".tsx", ".jsx", ".vue",
		".html", ".css", ".scss", ".go", ".rs", ".c", ".cpp", ".h", ".hpp",
		".java", ".kt", ".sql", ".xml", ".srt", ".smi", ".vtt", ".log":
		return "code"
	default:
		return "file"
	}
}

func (h *Handler) Routes(accessTier string) chi.Router {
	r := chi.NewRouter()
	r.Get("/roots", h.handleRoots(accessTier))
	r.Get("/list", h.handleList(accessTier))
	r.Post("/mkdir", h.handleMkdir(accessTier))
	r.Post("/rename", h.handleRename(accessTier))
	r.Post("/move", h.handleMove(accessTier))
	r.Post("/delete", h.handleDelete(accessTier))
	r.Get("/text", h.handleReadText(accessTier))
	r.Put("/text", h.handleWriteText(accessTier))
	r.Get("/download", h.handleDownload(accessTier))
	r.Get("/raw", h.handleRaw(accessTier))
	r.Post("/upload", h.handleMultipartUpload(accessTier))
	r.Post("/upload/init", h.handleUploadInit(accessTier))
	r.Patch("/upload/{uploadID}", h.handleUploadChunk)
	r.Post("/upload/{uploadID}/complete", h.handleUploadComplete(accessTier))
	return r
}

func writeJailError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnknownRoot):
		httputil.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrPathTraversal), errors.Is(err, ErrReadOnlyMount), errors.Is(err, ErrPrivateMount):
		httputil.WriteError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, os.ErrNotExist):
		httputil.WriteError(w, http.StatusNotFound, "파일 또는 폴더를 찾을 수 없습니다.")
	default:
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *Handler) handleRoots(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"roots": h.jail.RootsForTier(accessTier),
		})
	}
}

func (h *Handler) handleList(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rootID := r.URL.Query().Get("root")
		rel := r.URL.Query().Get("path")

		dirReal, cleanRel, err := h.jail.ResolveExistingForTier(rootID, rel, accessTier)
		if err != nil {
			writeJailError(w, err)
			return
		}
		info, err := os.Stat(dirReal)
		if err != nil {
			writeJailError(w, err)
			return
		}
		if !info.IsDir() {
			httputil.WriteError(w, http.StatusBadRequest, "디렉토리 경로가 아닙니다.")
			return
		}

		dirEntries, err := os.ReadDir(dirReal)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "디렉토리를 읽을 수 없습니다.")
			return
		}

		rootCfg, _ := h.jail.Root(rootID)
		items := make([]Entry, 0, len(dirEntries))
		for _, de := range dirEntries {
			name := de.Name()
			if strings.HasPrefix(name, ".ps-atomic-") || strings.HasPrefix(name, ".ps-upload-") {
				continue
			}
			fullCandidate := filepath.Join(dirReal, name)
			resolved, evalErr := filepath.EvalSymlinks(fullCandidate)
			if evalErr != nil || !isWithinRoot(resolved, rootCfg.Path) {
				// Hide broken symlinks or symlinks pointing outside the mounted root.
				continue
			}
			st, statErr := os.Stat(resolved)
			if statErr != nil {
				continue
			}
			entryRel := name
			if cleanRel != "" {
				entryRel = cleanRel + "/" + name
			}
			items = append(items, Entry{
				Name:    name,
				RelPath: entryRel,
				IsDir:   st.IsDir(),
				Size:    st.Size(),
				ModTime: st.ModTime().Unix(),
				Kind:    ClassifyFile(name, st.IsDir()),
			})
		}

		sort.Slice(items, func(i, j int) bool {
			if items[i].IsDir != items[j].IsDir {
				return items[i].IsDir
			}
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		})

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"rootId":   rootID,
			"relPath":  cleanRel,
			"readOnly": rootCfg.ReadOnly,
			"entries":  items,
		})
	}
}

func (h *Handler) handleMkdir(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RootID  string `json:"rootId"`
			RelPath string `json:"relPath"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청입니다.")
			return
		}
		target, cleanRel, err := h.jail.ResolveCreateTargetForTier(req.RootID, req.RelPath, accessTier)
		if err != nil {
			writeJailError(w, err)
			return
		}
		if err := os.Mkdir(target, 0o755); err != nil {
			if errors.Is(err, os.ErrExist) {
				httputil.WriteError(w, http.StatusConflict, "이미 같은 이름의 항목이 존재합니다.")
				return
			}
			httputil.WriteError(w, http.StatusInternalServerError, "폴더 생성에 실패했습니다.")
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, map[string]any{"relPath": cleanRel})
	}
}

func (h *Handler) handleRename(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RootID  string `json:"rootId"`
			FromRel string `json:"fromRel"`
			ToRel   string `json:"toRel"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청입니다.")
			return
		}
		if strings.TrimSpace(req.FromRel) == "" {
			httputil.WriteError(w, http.StatusBadRequest, "루트 디렉토리는 이름을 변경할 수 없습니다.")
			return
		}
		if err := h.jail.CheckWritable(req.RootID, accessTier); err != nil {
			writeJailError(w, err)
			return
		}
		srcReal, _, err := h.jail.ResolveExistingForTier(req.RootID, req.FromRel, accessTier)
		if err != nil {
			writeJailError(w, err)
			return
		}
		dstReal, cleanTo, err := h.jail.ResolveCreateTargetForTier(req.RootID, req.ToRel, accessTier)
		if err != nil {
			writeJailError(w, err)
			return
		}
		if _, err := os.Lstat(dstReal); err == nil {
			httputil.WriteError(w, http.StatusConflict, "대상 경로에 이미 파일이나 폴더가 있습니다.")
			return
		}
		if err := os.Rename(srcReal, dstReal); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "이름 변경 또는 이동에 실패했습니다.")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"relPath": cleanTo})
	}
}

func (h *Handler) handleMove(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			FromRootID string `json:"fromRootId"`
			FromRel    string `json:"fromRel"`
			ToRootID   string `json:"toRootId"`
			ToDirRel   string `json:"toDirRel"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청입니다.")
			return
		}
		if strings.TrimSpace(req.FromRel) == "" {
			httputil.WriteError(w, http.StatusBadRequest, "루트 디렉토리는 이동할 수 없습니다.")
			return
		}
		if err := h.jail.CheckWritable(req.FromRootID, accessTier); err != nil {
			writeJailError(w, err)
			return
		}
		if err := h.jail.CheckWritable(req.ToRootID, accessTier); err != nil {
			writeJailError(w, err)
			return
		}

		srcReal, srcCleanRel, err := h.jail.ResolveExistingForTier(req.FromRootID, req.FromRel, accessTier)
		if err != nil {
			writeJailError(w, err)
			return
		}
		srcInfo, err := os.Lstat(srcReal)
		if err != nil {
			writeJailError(w, err)
			return
		}

		dstDirReal, dstDirCleanRel, err := h.jail.ResolveExistingForTier(req.ToRootID, req.ToDirRel, accessTier)
		if err != nil {
			writeJailError(w, err)
			return
		}
		dstDirInfo, err := os.Stat(dstDirReal)
		if err != nil || !dstDirInfo.IsDir() {
			httputil.WriteError(w, http.StatusBadRequest, "이동 대상 경로가 폴더가 아닙니다.")
			return
		}

		leafName := filepath.Base(srcCleanRel)
		targetRel := leafName
		if dstDirCleanRel != "" {
			targetRel = dstDirCleanRel + "/" + leafName
		}

		dstReal, cleanTo, err := h.jail.ResolveCreateTargetForTier(req.ToRootID, targetRel, accessTier)
		if err != nil {
			writeJailError(w, err)
			return
		}

		if srcReal == dstReal {
			httputil.WriteJSON(w, http.StatusOK, map[string]any{
				"rootId":  req.ToRootID,
				"relPath": cleanTo,
				"moved":   false,
			})
			return
		}

		if srcInfo.IsDir() && strings.HasPrefix(dstReal, srcReal+string(os.PathSeparator)) {
			httputil.WriteError(w, http.StatusBadRequest, "폴더를 자기 자신의 하위 폴더로 이동할 수 없습니다.")
			return
		}

		if _, err := os.Lstat(dstReal); err == nil {
			httputil.WriteError(w, http.StatusConflict, "대상 폴더에 이미 같은 이름의 파일이나 폴더가 있습니다.")
			return
		}

		if err := moveEntryCrossDeviceSafe(srcReal, dstReal, srcInfo); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "파일 이동에 실패했습니다.")
			return
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"rootId":  req.ToRootID,
			"relPath": cleanTo,
			"moved":   true,
		})
	}
}

func moveEntryCrossDeviceSafe(srcPath, dstPath string, srcInfo os.FileInfo) error {
	if err := os.Rename(srcPath, dstPath); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	if !srcInfo.IsDir() {
		return moveFileCrossDeviceSafe(srcPath, dstPath)
	}

	if err := copyDirRecursive(srcPath, dstPath); err != nil {
		_ = os.RemoveAll(dstPath)
		return err
	}
	return os.RemoveAll(srcPath)
}

func copyDirRecursive(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, current)
		if err != nil {
			return err
		}
		target := dstDir
		if rel != "." {
			target = filepath.Join(dstDir, rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("심볼릭 링크는 다른 볼륨으로 이동할 수 없습니다")
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copySingleFile(current, target, info.Mode().Perm())
	})
}

func copySingleFile(srcPath, dstPath string, perm os.FileMode) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func (h *Handler) handleDelete(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RootID  string `json:"rootId"`
			RelPath string `json:"relPath"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청입니다.")
			return
		}
		if strings.TrimSpace(req.RelPath) == "" {
			httputil.WriteError(w, http.StatusForbidden, "루트 디렉토리는 삭제할 수 없습니다.")
			return
		}
		if err := h.jail.CheckWritable(req.RootID, accessTier); err != nil {
			writeJailError(w, err)
			return
		}
		targetReal, cleanRel, err := h.jail.ResolveExistingForTier(req.RootID, req.RelPath, accessTier)
		if err != nil {
			writeJailError(w, err)
			return
		}
		rootCfg, _ := h.jail.Root(req.RootID)
		if targetReal == rootCfg.Path || cleanRel == "" {
			httputil.WriteError(w, http.StatusForbidden, "루트 디렉토리는 삭제할 수 없습니다.")
			return
		}
		if err := os.RemoveAll(targetReal); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "삭제에 실패했습니다.")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (h *Handler) handleReadText(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rootID := r.URL.Query().Get("root")
		rel := r.URL.Query().Get("path")

		fileReal, cleanRel, err := h.jail.ResolveExistingForTier(rootID, rel, accessTier)
		if err != nil {
			writeJailError(w, err)
			return
		}
		st, err := os.Stat(fileReal)
		if err != nil {
			writeJailError(w, err)
			return
		}
		if st.IsDir() {
			httputil.WriteError(w, http.StatusBadRequest, "폴더는 텍스트 편집기로 열 수 없습니다.")
			return
		}
		if st.Size() > maxEditableSize {
			httputil.WriteError(w, http.StatusRequestEntityTooLarge, "5MB를 초과하는 파일은 편집기에서 열 수 없습니다.")
			return
		}

		raw, err := os.ReadFile(fileReal)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "파일을 읽을 수 없습니다.")
			return
		}
		probeLen := len(raw)
		if probeLen > 8192 {
			probeLen = 8192
		}
		if bytes.IndexByte(raw[:probeLen], 0) != -1 {
			httputil.WriteError(w, http.StatusUnsupportedMediaType, "바이너리 파일은 텍스트 편집기에서 열 수 없습니다.")
			return
		}

		rootCfg, _ := h.jail.Root(rootID)
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"rootId":   rootID,
			"relPath":  cleanRel,
			"name":     filepath.Base(fileReal),
			"content":  string(raw),
			"size":     st.Size(),
			"modTime":  st.ModTime().Unix(),
			"readOnly": rootCfg.ReadOnly,
		})
	}
}

func (h *Handler) handleWriteText(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RootID  string `json:"rootId"`
			RelPath string `json:"relPath"`
			Content string `json:"content"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxEditableSize+65536)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청이거나 파일 크기가 너무 큽니다.")
			return
		}
		targetReal, cleanRel, err := h.jail.ResolveCreateTargetForTier(req.RootID, req.RelPath, accessTier)
		if err != nil {
			writeJailError(w, err)
			return
		}
		if st, statErr := os.Stat(targetReal); statErr == nil && st.IsDir() {
			httputil.WriteError(w, http.StatusBadRequest, "폴더 경로에 파일을 덮어쓸 수 없습니다.")
			return
		}
		if err := AtomicWriteFile(targetReal, []byte(req.Content), 0o644); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "파일 저장에 실패했습니다.")
			return
		}
		st, _ := os.Stat(targetReal)
		var modTime int64
		var size int64
		if st != nil {
			modTime = st.ModTime().Unix()
			size = st.Size()
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"relPath": cleanRel,
			"size":    size,
			"modTime": modTime,
		})
	}
}

func setAttachmentHeader(w http.ResponseWriter, filename string) {
	escaped := url.PathEscape(filename)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, strings.ReplaceAll(filename, `"`, "_"), escaped))
}

func StreamDirAsZip(w http.ResponseWriter, dirReal, rootBoundary, zipFilename string) {
	w.Header().Set("Content-Type", "application/zip")
	setAttachmentHeader(w, zipFilename)
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	defer func() { _ = zw.Close() }()

	_ = filepath.WalkDir(dirReal, func(curr string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".ps-atomic-") || strings.HasPrefix(name, ".ps-upload-") {
			return nil
		}
		resolved, err := filepath.EvalSymlinks(curr)
		if err != nil || !isWithinRoot(resolved, rootBoundary) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dirReal, curr)
		if err != nil || rel == "." {
			return nil
		}
		zipRel := filepath.ToSlash(rel)
		info, err := os.Stat(resolved)
		if err != nil {
			return nil
		}
		if info.IsDir() {
			_, _ = zw.Create(zipRel + "/")
			return nil
		}

		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return nil
		}
		hdr.Name = zipRel
		hdr.Method = zip.Deflate

		writer, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		f, err := os.Open(resolved)
		if err != nil {
			return nil
		}
		_, _ = io.Copy(writer, f)
		_ = f.Close()
		return nil
	})
}

func (h *Handler) handleDownload(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rootID := r.URL.Query().Get("root")
		rel := r.URL.Query().Get("path")

		targetReal, cleanRel, err := h.jail.ResolveExistingForTier(rootID, rel, accessTier)
		if err != nil {
			writeJailError(w, err)
			return
		}
		st, err := os.Stat(targetReal)
		if err != nil {
			writeJailError(w, err)
			return
		}
		rootCfg, _ := h.jail.Root(rootID)

		if st.IsDir() {
			zipName := filepath.Base(targetReal) + ".zip"
			if cleanRel == "" {
				zipName = rootID + ".zip"
			}
			StreamDirAsZip(w, targetReal, rootCfg.Path, zipName)
			return
		}

		f, err := os.Open(targetReal)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "파일을 열 수 없습니다.")
			return
		}
		defer func() { _ = f.Close() }()

		setAttachmentHeader(w, st.Name())
		http.ServeContent(w, r, st.Name(), st.ModTime(), f)
	}
}

func (h *Handler) handleRaw(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rootID := r.URL.Query().Get("root")
		rel := r.URL.Query().Get("path")

		targetReal, _, err := h.jail.ResolveExistingForTier(rootID, rel, accessTier)
		if err != nil {
			writeJailError(w, err)
			return
		}
		st, err := os.Stat(targetReal)
		if err != nil || st.IsDir() {
			httputil.WriteError(w, http.StatusBadRequest, "유효한 파일이 아닙니다.")
			return
		}
		f, err := os.Open(targetReal)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "파일을 열 수 없습니다.")
			return
		}
		defer func() { _ = f.Close() }()
		http.ServeContent(w, r, st.Name(), st.ModTime(), f)
	}
}

func moveFileCrossDeviceSafe(srcPath, dstPath string) error {
	if err := os.Rename(srcPath, dstPath); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	tmp, err := os.CreateTemp(filepath.Dir(dstPath), ".ps-upload-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dstPath); err != nil {
		return err
	}
	cleanup = false
	_ = src.Close()
	_ = os.Remove(srcPath)
	return nil
}

func (h *Handler) handleMultipartUpload(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rootID := r.URL.Query().Get("root")
		dirRel := r.URL.Query().Get("path")

		if err := h.jail.CheckWritable(rootID, accessTier); err != nil {
			writeJailError(w, err)
			return
		}

		mr, err := r.MultipartReader()
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "멀티파트 요청이 아닙니다.")
			return
		}

		var uploaded []string
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				httputil.WriteError(w, http.StatusBadRequest, "업로드 스트림 읽기 오류: "+err.Error())
				return
			}
			fileName := filepath.Base(part.FileName())
			if fileName == "" || fileName == "." || fileName == ".." {
				_ = part.Close()
				continue
			}
			joinedRel := fileName
			if strings.TrimSpace(dirRel) != "" {
				joinedRel = strings.TrimSuffix(dirRel, "/") + "/" + fileName
			}
			dstReal, cleanRel, err := h.jail.ResolveCreateTargetForTier(rootID, joinedRel, accessTier)
			if err != nil {
				_ = part.Close()
				writeJailError(w, err)
				return
			}

			tmp, err := os.CreateTemp(filepath.Dir(dstReal), ".ps-upload-*")
			if err != nil {
				_ = part.Close()
				httputil.WriteError(w, http.StatusInternalServerError, "임시 파일 생성 실패")
				return
			}
			tmpName := tmp.Name()
			if _, copyErr := io.Copy(tmp, part); copyErr != nil {
				_ = tmp.Close()
				_ = os.Remove(tmpName)
				_ = part.Close()
				httputil.WriteError(w, http.StatusInternalServerError, "파일 기록 실패")
				return
			}
			_ = tmp.Chmod(0o644)
			_ = tmp.Close()
			_ = part.Close()

			if err := os.Rename(tmpName, dstReal); err != nil {
				_ = os.Remove(tmpName)
				httputil.WriteError(w, http.StatusInternalServerError, "파일 저장 실패")
				return
			}
			uploaded = append(uploaded, cleanRel)
		}

		httputil.WriteJSON(w, http.StatusCreated, map[string]any{"uploaded": uploaded})
	}
}

type uploadMeta struct {
	RootID    string `json:"rootId"`
	TargetRel string `json:"targetRel"`
	TotalSize int64  `json:"totalSize"`
	CreatedAt int64  `json:"createdAt"`
}

func isValidUploadID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (h *Handler) uploadPaths(uploadID string) (partPath, metaPath string) {
	return filepath.Join(h.tempUploadDir, uploadID+".part"), filepath.Join(h.tempUploadDir, uploadID+".json")
}

func (h *Handler) handleUploadInit(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RootID    string `json:"rootId"`
			DirPath   string `json:"dirPath"`
			FileName  string `json:"fileName"`
			TotalSize int64  `json:"totalSize"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청입니다.")
			return
		}
		base := filepath.Base(strings.TrimSpace(req.FileName))
		if base == "" || base == "." || base == ".." || req.TotalSize < 0 {
			httputil.WriteError(w, http.StatusBadRequest, "유효하지 않은 파일 이름 또는 크기입니다.")
			return
		}
		joinedRel := base
		if strings.TrimSpace(req.DirPath) != "" {
			joinedRel = strings.TrimSuffix(req.DirPath, "/") + "/" + base
		}
		if _, cleanRel, err := h.jail.ResolveCreateTargetForTier(req.RootID, joinedRel, accessTier); err != nil {
			writeJailError(w, err)
			return
		} else {
			joinedRel = cleanRel
		}

		rawID := make([]byte, 16)
		if _, err := rand.Read(rawID); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "업로드 세션 생성 실패")
			return
		}
		uploadID := hex.EncodeToString(rawID)
		partPath, metaPath := h.uploadPaths(uploadID)

		f, err := os.OpenFile(partPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "임시 업로드 파일 생성 실패")
			return
		}
		_ = f.Close()

		metaBytes, _ := json.Marshal(uploadMeta{
			RootID:    req.RootID,
			TargetRel: joinedRel,
			TotalSize: req.TotalSize,
			CreatedAt: time.Now().Unix(),
		})
		if err := os.WriteFile(metaPath, metaBytes, 0o644); err != nil {
			_ = os.Remove(partPath)
			httputil.WriteError(w, http.StatusInternalServerError, "업로드 메타데이터 기록 실패")
			return
		}

		httputil.WriteJSON(w, http.StatusCreated, map[string]any{
			"uploadId":  uploadID,
			"chunkSize": defaultChunkSz,
		})
	}
}

func (h *Handler) handleUploadChunk(w http.ResponseWriter, r *http.Request) {
	uploadID := chi.URLParam(r, "uploadID")
	if !isValidUploadID(uploadID) {
		httputil.WriteError(w, http.StatusBadRequest, "유효하지 않은 업로드 ID입니다.")
		return
	}
	offset, err := strconv.ParseInt(r.Header.Get("X-Upload-Offset"), 10, 64)
	if err != nil || offset < 0 {
		httputil.WriteError(w, http.StatusBadRequest, "유효하지 않은 X-Upload-Offset 헤더입니다.")
		return
	}

	partPath, metaPath := h.uploadPaths(uploadID)
	rawMeta, err := os.ReadFile(metaPath)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "업로드 세션을 찾을 수 없습니다.")
		return
	}
	var meta uploadMeta
	if err := json.Unmarshal(rawMeta, &meta); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "업로드 세션 메타데이터 오류")
		return
	}

	st, err := os.Stat(partPath)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "임시 업로드 파일을 찾을 수 없습니다.")
		return
	}
	if st.Size() != offset {
		httputil.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":          "오프셋이 일치하지 않습니다.",
			"expectedOffset": st.Size(),
		})
		return
	}

	f, err := os.OpenFile(partPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "업로드 파일 열기 실패")
		return
	}
	remaining := meta.TotalSize - offset
	written, copyErr := io.Copy(f, io.LimitReader(r.Body, remaining+1))
	_ = f.Close()
	if copyErr != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "청크 쓰기 실패")
		return
	}
	if offset+written > meta.TotalSize {
		_ = os.Remove(partPath)
		_ = os.Remove(metaPath)
		httputil.WriteError(w, http.StatusBadRequest, "전송된 데이터가 선언된 파일 크기를 초과했습니다.")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"offset": offset + written,
	})
}

func (h *Handler) handleUploadComplete(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uploadID := chi.URLParam(r, "uploadID")
		if !isValidUploadID(uploadID) {
			httputil.WriteError(w, http.StatusBadRequest, "유효하지 않은 업로드 ID입니다.")
			return
		}
		partPath, metaPath := h.uploadPaths(uploadID)
		rawMeta, err := os.ReadFile(metaPath)
		if err != nil {
			httputil.WriteError(w, http.StatusNotFound, "업로드 세션을 찾을 수 없습니다.")
			return
		}
		var meta uploadMeta
		if err := json.Unmarshal(rawMeta, &meta); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "업로드 메타데이터 파싱 실패")
			return
		}

		st, err := os.Stat(partPath)
		if err != nil {
			httputil.WriteError(w, http.StatusNotFound, "임시 업로드 파일을 찾을 수 없습니다.")
			return
		}
		if st.Size() != meta.TotalSize {
			httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("파일 크기가 일치하지 않습니다 (%d / %d).", st.Size(), meta.TotalSize))
			return
		}

		dstReal, cleanRel, err := h.jail.ResolveCreateTargetForTier(meta.RootID, meta.TargetRel, accessTier)
		if err != nil {
			_ = os.Remove(partPath)
			_ = os.Remove(metaPath)
			writeJailError(w, err)
			return
		}

		if err := moveFileCrossDeviceSafe(partPath, dstReal); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "최종 경로로 파일을 이동하지 못했습니다.")
			return
		}
		_ = os.Remove(metaPath)

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"relPath": cleanRel,
		})
	}
}
