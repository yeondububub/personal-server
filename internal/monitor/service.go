package monitor

import (
	"context"
	"math"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"

	"personal-server/internal/files"
	"personal-server/internal/httputil"
)

const sampleInterval = 2 * time.Second

type DiskStat struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	TotalBytes  uint64  `json:"totalBytes"`
	FreeBytes   uint64  `json:"freeBytes"`
	UsedBytes   uint64  `json:"usedBytes"`
	UsedPercent float64 `json:"usedPercent"`
}

type HostMetrics struct {
	Timestamp         int64     `json:"timestamp"`
	Hostname          string    `json:"hostname"`
	OS                string    `json:"os"`
	Platform          string    `json:"platform"`
	Arch              string    `json:"arch"`
	UptimeSeconds     uint64    `json:"uptimeSeconds"`
	AppUptimeSeconds  int64     `json:"appUptimeSeconds"`
	CPUCores          int       `json:"cpuCores"`
	CPUTotalPercent   float64   `json:"cpuTotalPercent"`
	CPUPerCore        []float64 `json:"cpuPerCore"`
	MemTotalBytes     uint64    `json:"memTotalBytes"`
	MemUsedBytes      uint64    `json:"memUsedBytes"`
	MemAvailableBytes uint64    `json:"memAvailableBytes"`
	MemUsedPercent    float64   `json:"memUsedPercent"`
	SwapTotalBytes    uint64    `json:"swapTotalBytes"`
	SwapUsedBytes     uint64    `json:"swapUsedBytes"`
	NetRxBytesPerSec  float64   `json:"netRxBytesPerSec"`
	NetTxBytesPerSec  float64   `json:"netTxBytesPerSec"`
	AppMemoryBytes    uint64    `json:"appMemoryBytes"`
	Goroutines        int       `json:"goroutines"`
}

type MonitorSnapshot struct {
	AccessTier string `json:"accessTier"`
	HostMetrics
	Disks []DiskStat `json:"disks"`
}

type Service struct {
	jail    *files.Jail
	started time.Time

	mu      sync.RWMutex
	latest  HostMetrics
	prevNet bool
	prevRx  uint64
	prevTx  uint64
	prevAt  time.Time

	subsMu      sync.Mutex
	subscribers map[chan struct{}]struct{}
	bgDone      chan struct{}
	bgOnce      sync.Once
}

func NewService(jail *files.Jail, started time.Time) *Service {
	if started.IsZero() {
		started = time.Now()
	}
	s := &Service{
		jail:        jail,
		started:     started,
		subscribers: make(map[chan struct{}]struct{}),
		bgDone:      make(chan struct{}),
	}
	s.collectSample()
	return s
}

func (s *Service) StartBackground(ctx context.Context) {
	s.bgOnce.Do(func() {
		go func() {
			defer close(s.bgDone)
			ticker := time.NewTicker(sampleInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s.collectSample()
					s.notifySubscribers()
				}
			}
		}()
	})
}

func (s *Service) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	s.subsMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subsMu.Unlock()
	return ch
}

func (s *Service) unsubscribe(ch chan struct{}) {
	s.subsMu.Lock()
	delete(s.subscribers, ch)
	s.subsMu.Unlock()
}

func (s *Service) notifySubscribers() {
	s.subsMu.Lock()
	for ch := range s.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	s.subsMu.Unlock()
}

func round1(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*10) / 10
}

func (s *Service) collectSample() {
	now := time.Now()
	m := HostMetrics{
		Timestamp:        now.UnixMilli(),
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		CPUCores:         runtime.NumCPU(),
		AppUptimeSeconds: int64(now.Sub(s.started).Seconds()),
		Goroutines:       runtime.NumGoroutine(),
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m.AppMemoryBytes = ms.Alloc

	if hInfo, err := host.Info(); err == nil {
		m.Hostname = hInfo.Hostname
		m.Platform = hInfo.Platform
		m.UptimeSeconds = hInfo.Uptime
	}

	if pcts, err := cpu.Percent(0, false); err == nil && len(pcts) > 0 {
		m.CPUTotalPercent = round1(pcts[0])
	}
	if perCore, err := cpu.Percent(0, true); err == nil && len(perCore) > 0 {
		rounded := make([]float64, len(perCore))
		for i, v := range perCore {
			rounded[i] = round1(v)
		}
		m.CPUPerCore = rounded
	} else {
		m.CPUPerCore = make([]float64, 0)
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		m.MemTotalBytes = vm.Total
		m.MemUsedBytes = vm.Used
		m.MemAvailableBytes = vm.Available
		m.MemUsedPercent = round1(vm.UsedPercent)
	}
	if sw, err := mem.SwapMemory(); err == nil {
		m.SwapTotalBytes = sw.Total
		m.SwapUsedBytes = sw.Used
	}

	var rxTotal, txTotal uint64
	hasNet := false
	if counters, err := gnet.IOCounters(false); err == nil && len(counters) > 0 {
		rxTotal = counters[0].BytesRecv
		txTotal = counters[0].BytesSent
		hasNet = true
	}

	s.mu.Lock()
	if hasNet {
		if s.prevNet {
			elapsed := now.Sub(s.prevAt).Seconds()
			if elapsed > 0 {
				if rxTotal >= s.prevRx {
					m.NetRxBytesPerSec = round1(float64(rxTotal-s.prevRx) / elapsed)
				}
				if txTotal >= s.prevTx {
					m.NetTxBytesPerSec = round1(float64(txTotal-s.prevTx) / elapsed)
				}
			}
		}
		s.prevNet = true
		s.prevRx = rxTotal
		s.prevTx = txTotal
		s.prevAt = now
	}
	s.latest = m
	s.mu.Unlock()
}

func (s *Service) Snapshot(accessTier string) MonitorSnapshot {
	s.mu.RLock()
	metrics := s.latest
	s.mu.RUnlock()

	disks := make([]DiskStat, 0)
	if s.jail != nil {
		roots := s.jail.RootsForTier(accessTier)
		disks = make([]DiskStat, 0, len(roots))
		for _, root := range roots {
			var st syscall.Statfs_t
			if err := syscall.Statfs(root.Path, &st); err == nil {
				bsize := uint64(st.Bsize)
				total := st.Blocks * bsize
				free := st.Bavail * bsize
				var used uint64
				if total >= free {
					used = total - free
				}
				var usedPct float64
				if total > 0 {
					usedPct = round1(float64(used) / float64(total) * 100)
				}
				disks = append(disks, DiskStat{
					ID:          root.ID,
					Name:        root.Name,
					TotalBytes:  total,
					FreeBytes:   free,
					UsedBytes:   used,
					UsedPercent: usedPct,
				})
			}
		}
	}

	return MonitorSnapshot{
		AccessTier:  accessTier,
		HostMetrics: metrics,
		Disks:       disks,
	}
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
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

func (s *Service) Routes(accessTier string) chi.Router {
	r := chi.NewRouter()
	r.Get("/snapshot", func(w http.ResponseWriter, _ *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, s.Snapshot(accessTier))
	})
	r.Get("/ws", s.handleWS(accessTier))
	return r
}

func (s *Service) handleWS(accessTier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteJSON(s.Snapshot(accessTier)); err != nil {
			return
		}

		clientClosed := make(chan struct{})
		go func() {
			defer close(clientClosed)
			for {
				if _, _, readErr := conn.ReadMessage(); readErr != nil {
					return
				}
			}
		}()

		sub := s.subscribe()
		defer s.unsubscribe(sub)

		fallback := time.NewTicker(sampleInterval)
		defer fallback.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-s.bgDone:
				return
			case <-clientClosed:
				return
			case <-sub:
				fallback.Reset(sampleInterval)
			case <-fallback.C:
			}

			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteJSON(s.Snapshot(accessTier)); err != nil {
				return
			}
		}
	}
}
