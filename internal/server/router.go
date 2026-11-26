package server

import (
	"io"
	"io/fs"
	"net/http"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"personal-server/internal/auth"
	"personal-server/internal/config"
	"personal-server/internal/files"
	"personal-server/internal/httputil"
	"personal-server/internal/media"
	"personal-server/internal/share"
)

type Dependencies struct {
	Config  config.Config
	Auth    *auth.Service
	Files   *files.Handler
	Media   *media.Handler
	Share   *share.Handler
	WebDist fs.FS
	Started time.Time
}

func NewPublicRouter(deps Dependencies) http.Handler {
	return buildRouter(deps, "public")
}

func NewPrivateRouter(deps Dependencies) http.Handler {
	return buildRouter(deps, "private")
}

func buildRouter(deps Dependencies, accessTier string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Route("/api", func(api chi.Router) {
		api.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			httputil.WriteJSON(w, http.StatusOK, map[string]any{
				"status":     "ok",
				"accessTier": accessTier,
			})
		})

		api.Post("/auth/login", deps.Auth.HandleLogin(accessTier))
		api.Post("/auth/logout", deps.Auth.HandleLogout)
		api.Get("/auth/me", deps.Auth.HandleMe(accessTier))

		api.Mount("/public/shares", deps.Share.PublicRoutes())

		api.Group(func(authed chi.Router) {
			authed.Use(deps.Auth.RequireAuth)
			authed.Post("/auth/password", deps.Auth.HandleChangePassword)
			authed.Mount("/files", deps.Files.Routes())
			authed.Mount("/media", deps.Media.Routes())
			authed.Mount("/shares", deps.Share.AdminRoutes())
			authed.Get("/system/summary", handleSystemSummary(deps, accessTier))

			if accessTier == "private" {
				authed.Get("/private/capabilities", func(w http.ResponseWriter, _ *http.Request) {
					httputil.WriteJSON(w, http.StatusOK, map[string]any{
						"wargameEnabled":  true,
						"terminalEnabled": true,
						"phase":           1,
					})
				})
			}
		})
	})

	spaHandler := makeSPAHandler(deps.WebDist)
	r.NotFound(spaHandler)
	return r
}

func handleSystemSummary(deps Dependencies, accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		type diskStat struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			TotalBytes uint64 `json:"totalBytes"`
			FreeBytes  uint64 `json:"freeBytes"`
		}
		disks := make([]diskStat, 0, len(deps.Config.Storage.Roots))
		for _, root := range deps.Config.Storage.Roots {
			var st syscall.Statfs_t
			if err := syscall.Statfs(root.Path, &st); err == nil {
				bsize := uint64(st.Bsize)
				disks = append(disks, diskStat{
					ID:         root.ID,
					Name:       root.Name,
					TotalBytes: st.Blocks * bsize,
					FreeBytes:  st.Bavail * bsize,
				})
			}
		}

		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"accessTier":     accessTier,
			"goOS":           runtime.GOOS,
			"goArch":         runtime.GOARCH,
			"cpuCores":       runtime.NumCPU(),
			"uptimeSeconds":  int64(time.Since(deps.Started).Seconds()),
			"appMemoryBytes": mem.Alloc,
			"disks":          disks,
		})
	}
}

func makeSPAHandler(webDist fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(webDist))
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			httputil.WriteError(w, http.StatusNotFound, "존재하지 않는 API 경로입니다.")
			return
		}
		cleanPath := strings.TrimPrefix(r.URL.Path, "/")
		if cleanPath != "" {
			if f, err := webDist.Open(cleanPath); err == nil {
				stat, statErr := f.Stat()
				_ = f.Close()
				if statErr == nil && !stat.IsDir() {
					fileServer.ServeHTTP(w, r)
					return
				}
			}
		}

		idx, err := webDist.Open("index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("프론트엔드 정적 빌드(web/dist)가 아직 준비되지 않았습니다. `make build`를 먼저 실행하세요."))
			return
		}
		defer func() { _ = idx.Close() }()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, idx)
	}
}
