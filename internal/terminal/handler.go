package terminal

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/creack/pty"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"personal-server/internal/httputil"
	"personal-server/internal/wargame"
)

var safeContainerPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

type Target struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label"`
}

type Handler struct {
	wg *wargame.Service
}

func NewHandler(wg *wargame.Service) *Handler {
	return &Handler{wg: wg}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/targets", h.handleTargets)
	r.Get("/ws", h.handleWS)
	return r
}

func ValidateContainerName(name string) bool {
	if !strings.HasPrefix(name, "ps-wg-") || len(name) <= len("ps-wg-") {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	return safeContainerPattern.MatchString(name)
}

func (h *Handler) handleTargets(w http.ResponseWriter, _ *http.Request) {
	targets := []Target{
		{
			Kind:  "host",
			ID:    "host",
			Label: "호스트 쉘",
		},
	}

	if h.wg != nil {
		if instances, err := h.wg.ListInstances(); err == nil {
			for _, inst := range instances {
				if inst.Status != "running" || inst.Mode == "compose" {
					continue
				}
				targets = append(targets, Target{
					Kind:  "docker",
					ID:    "ps-wg-" + inst.ID,
					Label: "워게임: " + inst.Name,
				})
			}
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"targets": targets,
	})
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return strings.EqualFold(u.Host, r.Host)
	},
}

func parseDim(raw string, def, minVal, maxVal int) uint16 {
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		v = def
	}
	if v < minVal {
		v = minVal
	}
	if v > maxVal {
		v = maxVal
	}
	return uint16(v)
}

func resolveHostShell() string {
	candidates := []string{os.Getenv("SHELL"), "/bin/bash", "/bin/sh"}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return "/bin/sh"
}

type clientMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func (h *Handler) handleWS(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.URL.Query().Get("target"))
	if target == "" {
		target = "host"
	}
	container := strings.TrimSpace(r.URL.Query().Get("container"))
	cols := parseDim(r.URL.Query().Get("cols"), 100, 10, 500)
	rows := parseDim(r.URL.Query().Get("rows"), 30, 5, 200)

	var cmd *exec.Cmd
	switch target {
	case "host":
		shell := resolveHostShell()
		cmd = exec.Command(shell, "-l")
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	case "docker":
		if !ValidateContainerName(container) {
			httputil.WriteError(w, http.StatusBadRequest, "유효하지 않은 컨테이너 이름입니다.")
			return
		}
		cmd = exec.Command(
			"docker", "exec", "-it",
			"-e", "TERM=xterm-256color",
			container,
			"/bin/sh", "-c", "command -v bash >/dev/null 2>&1 && exec bash || exec sh",
		)
	default:
		httputil.WriteError(w, http.StatusBadRequest, "지원하지 않는 터미널 대상입니다.")
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("터미널 시작 실패: "+err.Error()+"\r\n"))
		_ = conn.Close()
		return
	}

	var (
		writeMu     sync.Mutex
		cleanupOnce sync.Once
		wg          sync.WaitGroup
	)

	cleanup := func() {
		cleanupOnce.Do(func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = ptmx.Close()
			_ = conn.Close()
		})
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = cmd.Wait()
		cleanup()
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				writeMu.Lock()
				wErr := conn.WriteMessage(websocket.BinaryMessage, buf[:n])
				writeMu.Unlock()
				if wErr != nil {
					cleanup()
					return
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					writeMu.Lock()
					_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
					writeMu.Unlock()
				}
				cleanup()
				return
			}
		}
	}()

	for {
		_, payload, readErr := conn.ReadMessage()
		if readErr != nil {
			break
		}
		var msg clientMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "input":
			if msg.Data != "" {
				if _, err := ptmx.Write([]byte(msg.Data)); err != nil {
					cleanup()
					wg.Wait()
					return
				}
			}
		case "resize":
			if msg.Cols > 0 && msg.Rows > 0 {
				c := parseDim(strconv.Itoa(msg.Cols), int(cols), 10, 500)
				rw := parseDim(strconv.Itoa(msg.Rows), int(rows), 5, 200)
				_ = pty.Setsize(ptmx, &pty.Winsize{Cols: c, Rows: rw})
			}
		}
	}

	cleanup()
	wg.Wait()
}
