package terminal

import (
	"context"
	"encoding/base64"
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
	"time"

	"github.com/creack/pty"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"personal-server/internal/httputil"
	"personal-server/internal/wargame"
)

var safeContainerPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

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
	if len(name) < 1 || len(name) > 128 || strings.Contains(name, "..") {
		return false
	}
	return safeContainerPattern.MatchString(name)
}

func (h *Handler) handleTargets(w http.ResponseWriter, r *http.Request) {
	targets := []Target{
		{
			Kind:  "host",
			ID:    "host",
			Label: "호스트 쉘",
		},
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "ps", "--format", "{{.Names}}\t{{.Labels}}").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			name := strings.TrimPrefix(strings.TrimSpace(parts[0]), "/")
			if !ValidateContainerName(name) {
				continue
			}
			labels := ""
			if len(parts) > 1 {
				labels = parts[1]
			}
			label := "컨테이너: " + name
			if strings.Contains(labels, "personal-server.wargame.id=") {
				label = "워게임: " + name
			}
			targets = append(targets, Target{
				Kind:  "docker",
				ID:    name,
				Label: label,
			})
		}
	} else if h.wg != nil {
		if instances, wgErr := h.wg.ListInstances(); wgErr == nil {
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

func buildHostTerminalEnv() []string {
	env := os.Environ()
	hasLang := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "LANG=") && len(kv) > len("LANG=") {
			hasLang = true
			break
		}
	}
	env = append(env, "TERM=xterm-256color", "COLORTERM=truecolor")
	if !hasLang {
		env = append(env, "LANG=C.UTF-8")
	}
	return env
}

const (
	wsPingInterval = 25 * time.Second
	wsWriteTimeout = 10 * time.Second
)

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
		cmd.Env = buildHostTerminalEnv()
	case "docker":
		if !ValidateContainerName(container) {
			httputil.WriteError(w, http.StatusBadRequest, "유효하지 않은 컨테이너 이름입니다.")
			return
		}
		cmd = exec.Command(
			"docker", "exec", "-it",
			"-e", "TERM=xterm-256color",
			"-e", "COLORTERM=truecolor",
			container,
			"/bin/sh", "-c", "stty -ixon -ixoff 2>/dev/null || true; command -v bash >/dev/null 2>&1 && exec bash || exec sh",
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
	disableFlowControl(ptmx)

	var (
		writeMu     sync.Mutex
		cleanupOnce sync.Once
		wg          sync.WaitGroup
		stopPing    = make(chan struct{})
	)

	cleanup := func() {
		cleanupOnce.Do(func() {
			close(stopPing)
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = ptmx.Close()
			_ = conn.Close()
		})
	}

	wg.Add(3)
	go func() {
		defer wg.Done()
		_ = cmd.Wait()
		cleanup()
	}()

	go func() {
		defer wg.Done()
		ticker := time.NewTicker(wsPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-ticker.C:
				writeMu.Lock()
				_ = conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
				err := conn.WriteMessage(websocket.PingMessage, nil)
				_ = conn.SetWriteDeadline(time.Time{})
				writeMu.Unlock()
				if err != nil {
					cleanup()
					return
				}
			}
		}
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				writeMu.Lock()
				_ = conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
				wErr := conn.WriteMessage(websocket.BinaryMessage, buf[:n])
				_ = conn.SetWriteDeadline(time.Time{})
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
		case "binary":
			if msg.Data != "" {
				decoded, decErr := base64.StdEncoding.DecodeString(msg.Data)
				if decErr == nil && len(decoded) > 0 {
					if _, err := ptmx.Write(decoded); err != nil {
						cleanup()
						wg.Wait()
						return
					}
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
