package wargame

import (
	"errors"
	"net"
	"strconv"
	"sync"
)

var ErrNoAvailablePorts = errors.New("할당 가능한 호스트 포트가 부족합니다")

type PortAllocator struct {
	mu     sync.Mutex
	bindIP string
	start  int
	end    int
	inUse  map[int]string
}

func NewPortAllocator(bindIP string, start, end int) *PortAllocator {
	if bindIP == "" {
		bindIP = "127.0.0.1"
	}
	if start <= 0 {
		start = 30000
	}
	if end <= start {
		end = start + 9999
	}
	return &PortAllocator{
		bindIP: bindIP,
		start:  start,
		end:    end,
		inUse:  make(map[int]string),
	}
}

func (a *PortAllocator) MarkOwned(instanceID string, ports []PortBinding) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, pb := range ports {
		if pb.HostPort >= a.start && pb.HostPort <= a.end {
			a.inUse[pb.HostPort] = instanceID
		}
	}
}

func (a *PortAllocator) ReleaseInstance(instanceID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for port, owner := range a.inUse {
		if owner == instanceID {
			delete(a.inUse, port)
		}
	}
}

func (a *PortAllocator) ReserveForInstance(instanceID string, bindings []PortBinding) ([]PortBinding, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	out := make([]PortBinding, len(bindings))
	copy(out, bindings)

	newlyReserved := make([]int, 0, len(out))
	batchUsed := make(map[int]struct{}, len(out))

	for i := range out {
		preferred := out[i].HostPort
		if preferred >= a.start && preferred <= a.end {
			if _, dupInBatch := batchUsed[preferred]; !dupInBatch {
				owner, taken := a.inUse[preferred]
				if owner == instanceID {
					batchUsed[preferred] = struct{}{}
					continue
				}
				if !taken && isTCPPortFree(a.bindIP, preferred) {
					a.inUse[preferred] = instanceID
					newlyReserved = append(newlyReserved, preferred)
					batchUsed[preferred] = struct{}{}
					continue
				}
			}
		}

		allocated := 0
		for p := a.start; p <= a.end; p++ {
			if _, taken := a.inUse[p]; taken {
				continue
			}
			if _, dupInBatch := batchUsed[p]; dupInBatch {
				continue
			}
			if !isTCPPortFree(a.bindIP, p) {
				continue
			}
			allocated = p
			break
		}
		if allocated == 0 {
			for _, p := range newlyReserved {
				delete(a.inUse, p)
			}
			return nil, ErrNoAvailablePorts
		}

		a.inUse[allocated] = instanceID
		newlyReserved = append(newlyReserved, allocated)
		batchUsed[allocated] = struct{}{}
		out[i].HostPort = allocated
	}

	// Release any stale port previously held by this instance that is no longer in its binding list.
	for port, owner := range a.inUse {
		if owner == instanceID {
			if _, stillUsed := batchUsed[port]; !stillUsed {
				delete(a.inUse, port)
			}
		}
	}

	return out, nil
}

func isTCPPortFree(bindIP string, port int) bool {
	addr := net.JoinHostPort(bindIP, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
