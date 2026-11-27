package docker

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"personal-server/internal/httputil"
)

func (s *Service) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/overview", s.handleOverview)

	r.Get("/containers", s.handleListContainers)
	r.Post("/containers/{id}/{action}", s.handleContainerAction)
	r.Delete("/containers/{id}", s.handleDeleteContainer)
	r.Get("/containers/{id}/logs", s.handleContainerLogs)
	r.Get("/containers/{id}/inspect", s.handleContainerInspect)

	r.Get("/images", s.handleListImages)
	r.Post("/images/pull", s.handlePullImage)
	r.Post("/images/prune", s.handlePruneImages)
	r.Delete("/images/{id}", s.handleDeleteImage)

	r.Get("/stacks", s.handleListStacks)
	r.Get("/stacks/{name}", s.handleGetStack)
	r.Put("/stacks/{name}", s.handleSaveStack)
	r.Post("/stacks/{name}/{action}", s.handleStackAction)
	r.Get("/stacks/{name}/logs", s.handleStackLogs)
	r.Delete("/stacks/{name}", s.handleDeleteStack)

	r.Get("/volumes", s.handleListVolumes)
	r.Delete("/volumes/{name}", s.handleDeleteVolume)

	r.Get("/networks", s.handleListNetworks)
	r.Delete("/networks/{id}", s.handleDeleteNetwork)

	return r
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidRef),
		errors.Is(err, ErrInvalidAction),
		errors.Is(err, ErrInvalidYAML),
		errors.Is(err, ErrProtectedNetwork):
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, os.ErrNotExist):
		httputil.WriteError(w, http.StatusNotFound, "대상을 찾을 수 없습니다.")
	default:
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func (s *Service) handleOverview(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, s.Overview(r.Context()))
}

func (s *Service) handleListContainers(w http.ResponseWriter, r *http.Request) {
	containers, err := s.ListContainers(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"containers": containers})
}

func (s *Service) handleContainerAction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	action := chi.URLParam(r, "action")
	if err := s.ContainerAction(r.Context(), id, action); err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleDeleteContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	if err := s.RemoveContainer(r.Context(), id, force); err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tail := 300
	if raw := strings.TrimSpace(r.URL.Query().Get("tail")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			tail = parsed
		}
	}
	logs, err := s.ContainerLogs(r.Context(), id, tail)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (s *Service) handleContainerInspect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	info, err := s.InspectContainer(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, info)
}

func (s *Service) handleListImages(w http.ResponseWriter, r *http.Request) {
	images, err := s.ListImages(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"images": images})
}

func (s *Service) handlePullImage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Image string `json:"image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청 본문입니다.")
		return
	}
	output, err := s.PullImage(r.Context(), strings.TrimSpace(body.Image))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "output": output})
}

func (s *Service) handleDeleteImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	if err := s.RemoveImage(r.Context(), id, force); err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handlePruneImages(w http.ResponseWriter, r *http.Request) {
	output, err := s.PruneImages(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "output": output})
}

func (s *Service) handleListStacks(w http.ResponseWriter, r *http.Request) {
	stacks, err := s.ListStacks(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"stacks": stacks})
}

func (s *Service) handleGetStack(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	composeYaml, err := s.GetStack(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"name":        name,
		"composeYaml": composeYaml,
	})
}

func (s *Service) handleSaveStack(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var body struct {
		ComposeYaml string `json:"composeYaml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "잘못된 요청 본문입니다.")
		return
	}
	if err := s.SaveStack(name, body.ComposeYaml); err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleStackAction(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	action := chi.URLParam(r, "action")
	output, err := s.StackAction(r.Context(), name, action)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "output": output})
}

func (s *Service) handleStackLogs(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	logs, err := s.StackLogs(r.Context(), name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (s *Service) handleDeleteStack(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := s.DeleteStack(r.Context(), name); err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleListVolumes(w http.ResponseWriter, r *http.Request) {
	volumes, err := s.ListVolumes(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"volumes": volumes})
}

func (s *Service) handleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := s.RemoveVolume(r.Context(), name); err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Service) handleListNetworks(w http.ResponseWriter, r *http.Request) {
	networks, err := s.ListNetworks(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"networks": networks})
}

func (s *Service) handleDeleteNetwork(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.RemoveNetwork(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
