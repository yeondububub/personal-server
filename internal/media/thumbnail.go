package media

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"personal-server/internal/httputil"
)

func thumbnailCacheKey(realPath string, modTime time.Time, size int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", realPath, modTime.UnixNano(), size)))
	return hex.EncodeToString(sum[:])
}

func generateThumbnail(ctx context.Context, realPath, outPath string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}

	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	tmpPath := outPath + "." + hex.EncodeToString(suffix) + ".tmp.jpg"
	defer func() { _ = os.Remove(tmpPath) }()

	runAt := func(seek string) bool {
		cmdCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		err := exec.CommandContext(
			cmdCtx,
			"ffmpeg",
			"-y",
			"-ss", seek,
			"-i", realPath,
			"-frames:v", "1",
			"-vf", "scale=320:-1",
			"-q:v", "5",
			tmpPath,
		).Run()
		if err != nil {
			return false
		}
		st, statErr := os.Stat(tmpPath)
		return statErr == nil && st.Size() > 0
	}

	if !runAt("5") && !runAt("0") {
		return fmt.Errorf("ffmpeg thumbnail extraction failed")
	}
	return os.Rename(tmpPath, outPath)
}

func (h *Handler) handleThumbnail(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rootID := r.URL.Query().Get("root")
		rel := r.URL.Query().Get("path")

		realPath, _, err := h.jail.ResolveExistingForTier(rootID, rel, accessTier)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		st, err := os.Stat(realPath)
		if err != nil || st.IsDir() {
			httputil.WriteError(w, http.StatusNotFound, "영상 파일을 찾을 수 없습니다.")
			return
		}

		cachePath := filepath.Join(h.thumbsDir, thumbnailCacheKey(realPath, st.ModTime(), st.Size())+".jpg")
		if cached, statErr := os.Stat(cachePath); statErr != nil || cached.Size() == 0 {
			if genErr := generateThumbnail(r.Context(), realPath, cachePath); genErr != nil {
				httputil.WriteError(w, http.StatusInternalServerError, "썸네일 생성에 실패했습니다.")
				return
			}
		}

		f, err := os.Open(cachePath)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "썸네일을 열 수 없습니다.")
			return
		}
		defer func() { _ = f.Close() }()

		cst, err := f.Stat()
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "썸네일을 읽을 수 없습니다.")
			return
		}

		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeContent(w, r, filepath.Base(cachePath), cst.ModTime(), f)
	}
}
