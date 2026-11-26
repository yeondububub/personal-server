package wargame

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"personal-server/internal/files"
	"personal-server/internal/httputil"
)

func (s *Service) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", s.handleList)
	r.Post("/prepare/upload", s.handlePrepareUpload)
	r.Post("/prepare/mount", s.handlePrepareMount)
	r.Post("/{id}/start", s.handleStart)
	r.Post("/{id}/reset", s.handleReset)
	r.Post("/{id}/stop", s.handleStop)
	r.Delete("/{id}", s.handleDelete)
	r.Post("/{id}/extend", s.handleExtend)
	r.Get("/{id}/logs", s.handleLogs)
	r.Get("/{id}/source", s.handleSource)
	return r
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	dockerAvail, dockerErr := s.DockerStatus(r.Context())
	instances, err := s.ListInstances()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "워게임 인스턴스 목록을 불러오지 못했습니다.")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"dockerAvailable": dockerAvail,
		"dockerError":     dockerErr,
		"bindIp":          s.cfg.BindIP,
		"instances":       instances,
	})
}

func (s *Service) handlePrepareUpload(w http.ResponseWriter, r *http.Request) {
	mr, err := r.MultipartReader()
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "멀티파트 업로드 요청이 아닙니다.")
		return
	}

	var tmpPath string
	var origFilename string
	var customName string
	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()

	for {
		part, nextErr := mr.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			httputil.WriteError(w, http.StatusBadRequest, "업로드 스트림 읽기 실패: "+nextErr.Error())
			return
		}

		formName := part.FormName()
		switch formName {
		case "name":
			raw, _ := io.ReadAll(io.LimitReader(part, 1024))
			customName = strings.TrimSpace(string(raw))
			_ = part.Close()

		case "file":
			fn := filepath.Base(strings.TrimSpace(part.FileName()))
			if !IsSupportedArchive(fn) {
				_ = part.Close()
				httputil.WriteError(w, http.StatusBadRequest, ErrUnsupportedArchive.Error())
				return
			}
			origFilename = fn

			suffix := archiveExtSuffix(fn)
			tmpFile, createErr := os.CreateTemp(s.cfg.WorkDir, ".ps-wg-upload-*"+suffix)
			if createErr != nil {
				_ = part.Close()
				httputil.WriteError(w, http.StatusInternalServerError, "임시 아카이브 파일 생성 실패")
				return
			}
			tmpPath = tmpFile.Name()
			written, copyErr := io.Copy(tmpFile, io.LimitReader(part, s.cfg.MaxArchiveBytes+1))
			_ = tmpFile.Close()
			_ = part.Close()
			if copyErr != nil {
				httputil.WriteError(w, http.StatusInternalServerError, "아카이브 업로드 기록 실패")
				return
			}
			if written > s.cfg.MaxArchiveBytes {
				httputil.WriteError(w, http.StatusRequestEntityTooLarge, ErrArchiveTooLarge.Error())
				return
			}

		default:
			_ = part.Close()
		}
	}

	if tmpPath == "" {
		httputil.WriteError(w, http.StatusBadRequest, "업로드할 아카이브 파일(file)이 필요합니다.")
		return
	}

	inst, err := s.PrepareFromArchive(r.Context(), tmpPath, origFilename, customName)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"instance": inst})
}

func archiveExtSuffix(filename string) string {
	lower := strings.ToLower(filename)
	for _, ext := range []string{".tar.gz", ".tar.xz", ".tgz", ".tar", ".zip"} {
		if strings.HasSuffix(lower, ext) {
			return ext
		}
	}
	return filepath.Ext(filename)
}

func (s *Service) handlePrepareMount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RootID  string `json:"rootId"`
		RelPath string `json:"relPath"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청 형식입니다.")
		return
	}
	if strings.TrimSpace(req.RootID) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "마운트 루트 ID(rootId)가 필요합니다.")
		return
	}

	inst, err := s.PrepareFromMount(r.Context(), req.RootID, req.RelPath, req.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"instance": inst})
}

func (s *Service) handleStart(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var opts StartOptions
	if r.Body != nil && r.Body != http.NoBody {
		decErr := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&opts)
		if decErr != nil && !errors.Is(decErr, io.EOF) {
			httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청 형식입니다.")
			return
		}
	}

	inst, err := s.Start(r.Context(), id, opts)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{"instance": inst})
}

func (s *Service) handleReset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	inst, err := s.Reset(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"instance": inst})
}

func (s *Service) handleStop(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	inst, err := s.Stop(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"instance": inst})
}

func (s *Service) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Service) handleExtend(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Hours int `json:"hours"`
	}
	if r.Body != nil && r.Body != http.NoBody {
		_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req)
	}
	inst, err := s.Extend(id, req.Hours)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"instance": inst})
}

func (s *Service) handleLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	buildLog, containerLog, status, err := s.Logs(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"buildLog":     buildLog,
		"containerLog": containerLog,
		"status":       status,
	})
}

func (s *Service) handleSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	srcReal, boundaryReal, downloadName, err := s.SourceDir(id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	files.StreamDirAsZip(w, srcReal, boundaryReal, downloadName)
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, os.ErrNotExist):
		httputil.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInvalidState):
		httputil.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrDockerUnavailable), errors.Is(err, ErrNoAvailablePorts):
		httputil.WriteError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ErrArchiveTooLarge):
		httputil.WriteError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, ErrBadRequest),
		errors.Is(err, ErrUnsafeArchiveEntry),
		errors.Is(err, ErrUnsupportedArchive),
		errors.Is(err, files.ErrUnknownRoot),
		errors.Is(err, files.ErrPathTraversal):
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, files.ErrPrivateMount):
		httputil.WriteError(w, http.StatusForbidden, err.Error())
	default:
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}
