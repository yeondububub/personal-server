package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"personal-server/internal/files"
)

var (
	ErrInvalidRef        = errors.New("유효하지 않은 Docker 대상 식별자입니다")
	ErrInvalidAction     = errors.New("지원하지 않는 작업입니다")
	ErrInvalidYAML       = errors.New("유효한 YAML 형식이 아닙니다")
	ErrProtectedNetwork  = errors.New("기본 Docker 네트워크(bridge, host, none)는 삭제할 수 없습니다")
	containerRefPattern  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	imageRefPattern      = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:/@-]*$`)
	stackNamePattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	volumeNetworkPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
)

func ValidateContainerRef(id string) bool {
	if len(id) < 1 || len(id) > 128 || strings.Contains(id, "..") {
		return false
	}
	return containerRefPattern.MatchString(id)
}

func ValidateImageRef(ref string) bool {
	if len(ref) < 1 || len(ref) > 256 || strings.Contains(ref, "..") {
		return false
	}
	return imageRefPattern.MatchString(ref)
}

func ValidateStackName(name string) bool {
	if len(name) < 1 || len(name) > 64 {
		return false
	}
	return stackNamePattern.MatchString(name)
}

func ValidateVolumeOrNetwork(ref string) bool {
	if len(ref) < 1 || len(ref) > 128 || strings.Contains(ref, "..") {
		return false
	}
	return volumeNetworkPattern.MatchString(ref)
}

type Service struct {
	stacksDir string
}

func NewService(stacksDir string) (*Service, error) {
	if strings.TrimSpace(stacksDir) == "" {
		stacksDir = "./data/stacks"
	}
	if err := os.MkdirAll(stacksDir, 0o755); err != nil {
		return nil, fmt.Errorf("create stacks dir: %w", err)
	}
	abs, err := filepath.Abs(stacksDir)
	if err != nil {
		return nil, fmt.Errorf("abs stacks dir: %w", err)
	}
	if realPath, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
		abs = realPath
	}
	return &Service{stacksDir: abs}, nil
}

type DiskUsageItem struct {
	Type        string `json:"type"`
	TotalCount  string `json:"totalCount"`
	Active      string `json:"active"`
	Size        string `json:"size"`
	Reclaimable string `json:"reclaimable"`
}

type Overview struct {
	Available         bool            `json:"available"`
	Error             string          `json:"error,omitempty"`
	ServerVersion     string          `json:"serverVersion"`
	ContainersRunning int             `json:"containersRunning"`
	ContainersStopped int             `json:"containersStopped"`
	ImagesCount       int             `json:"imagesCount"`
	VolumesCount      int             `json:"volumesCount"`
	NetworksCount     int             `json:"networksCount"`
	DiskUsage         []DiskUsageItem `json:"diskUsage"`
}

type ContainerInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Image      string `json:"image"`
	State      string `json:"state"`
	Status     string `json:"status"`
	Ports      string `json:"ports"`
	CreatedAt  string `json:"createdAt"`
	IsWargame  bool   `json:"isWargame"`
	CPUPercent string `json:"cpuPercent"`
	MemUsage   string `json:"memUsage"`
	MemPercent string `json:"memPercent"`
}

type ContainerStat struct {
	ID         string
	Container  string
	Name       string
	CPUPercent string
	MemUsage   string
	MemPercent string
}

type MountInfo struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	RW          bool   `json:"rw"`
}

type NetworkAttachment struct {
	Name      string `json:"name"`
	IPAddress string `json:"ipAddress"`
}

type ContainerInspect struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Image         string              `json:"image"`
	State         string              `json:"state"`
	RestartPolicy string              `json:"restartPolicy"`
	Env           []string            `json:"env"`
	Mounts        []MountInfo         `json:"mounts"`
	Networks      []NetworkAttachment `json:"networks"`
	Ports         []string            `json:"ports"`
}

type ImageInfo struct {
	ID           string `json:"id"`
	Repository   string `json:"repository"`
	Tag          string `json:"tag"`
	Size         string `json:"size"`
	CreatedSince string `json:"createdSince"`
	InUse        bool   `json:"inUse"`
}

type StackSummary struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	RunningCount int    `json:"runningCount"`
	TotalCount   int    `json:"totalCount"`
	UpdatedAt    int64  `json:"updatedAt"`
}

type VolumeInfo struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	Mountpoint string `json:"mountpoint"`
	Scope      string `json:"scope"`
}

type NetworkInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Scope  string `json:"scope"`
}

func runDockerStdout(ctx context.Context, timeout time.Duration, dir string, args ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "docker", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return stdout.String(), nil
}

func runDockerCombined(ctx context.Context, timeout time.Duration, dir string, args ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "docker", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		msg := strings.TrimSpace(text)
		if msg == "" {
			msg = err.Error()
		}
		return text, errors.New(msg)
	}
	return text, nil
}

func countNonEmptyLines(raw string) int {
	n := 0
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func ParseDiskUsageNDJSON(raw string) []DiskUsageItem {
	items := []DiskUsageItem{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			Type        string `json:"Type"`
			TotalCount  string `json:"TotalCount"`
			Active      string `json:"Active"`
			Size        string `json:"Size"`
			Reclaimable string `json:"Reclaimable"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		items = append(items, DiskUsageItem{
			Type:        row.Type,
			TotalCount:  row.TotalCount,
			Active:      row.Active,
			Size:        row.Size,
			Reclaimable: row.Reclaimable,
		})
	}
	return items
}

func ParseContainersNDJSON(raw string) []ContainerInfo {
	out := []ContainerInfo{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			ID        string `json:"ID"`
			Names     string `json:"Names"`
			Image     string `json:"Image"`
			State     string `json:"State"`
			Status    string `json:"Status"`
			Ports     string `json:"Ports"`
			CreatedAt string `json:"CreatedAt"`
			Labels    string `json:"Labels"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		out = append(out, ContainerInfo{
			ID:        row.ID,
			Name:      strings.TrimPrefix(row.Names, "/"),
			Image:     row.Image,
			State:     row.State,
			Status:    row.Status,
			Ports:     row.Ports,
			CreatedAt: row.CreatedAt,
			IsWargame: strings.Contains(row.Labels, "personal-server.wargame.id="),
		})
	}
	return out
}

func ParseStatsNDJSON(raw string) map[string]ContainerStat {
	stats := make(map[string]ContainerStat)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			ID        string `json:"ID"`
			Container string `json:"Container"`
			Name      string `json:"Name"`
			CPUPerc   string `json:"CPUPerc"`
			MemUsage  string `json:"MemUsage"`
			MemPerc   string `json:"MemPerc"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		st := ContainerStat{
			ID:         row.ID,
			Container:  row.Container,
			Name:       strings.TrimPrefix(row.Name, "/"),
			CPUPercent: row.CPUPerc,
			MemUsage:   row.MemUsage,
			MemPercent: row.MemPerc,
		}
		for _, key := range []string{st.ID, st.Container, st.Name} {
			if key != "" {
				stats[key] = st
			}
		}
	}
	return stats
}

func MergeContainerStats(containers []ContainerInfo, stats map[string]ContainerStat) {
	for i := range containers {
		c := &containers[i]
		if st, ok := stats[c.ID]; ok {
			c.CPUPercent = st.CPUPercent
			c.MemUsage = st.MemUsage
			c.MemPercent = st.MemPercent
			continue
		}
		if len(c.ID) >= 12 {
			if st, ok := stats[c.ID[:12]]; ok {
				c.CPUPercent = st.CPUPercent
				c.MemUsage = st.MemUsage
				c.MemPercent = st.MemPercent
				continue
			}
		}
		if st, ok := stats[c.Name]; ok {
			c.CPUPercent = st.CPUPercent
			c.MemUsage = st.MemUsage
			c.MemPercent = st.MemPercent
		}
	}
}

func ParseContainerInspect(raw string) (*ContainerInspect, error) {
	var rows []struct {
		ID    string `json:"Id"`
		Name  string `json:"Name"`
		State struct {
			Status string `json:"Status"`
		} `json:"State"`
		Config struct {
			Image string   `json:"Image"`
			Env   []string `json:"Env"`
		} `json:"Config"`
		HostConfig struct {
			RestartPolicy struct {
				Name string `json:"Name"`
			} `json:"RestartPolicy"`
		} `json:"HostConfig"`
		Mounts []struct {
			Type        string `json:"Type"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
			RW          bool   `json:"RW"`
		} `json:"Mounts"`
		NetworkSettings struct {
			Networks map[string]struct {
				IPAddress string `json:"IPAddress"`
			} `json:"Networks"`
			Ports map[string][]struct {
				HostIP   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			} `json:"Ports"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, fmt.Errorf("parse docker inspect: %w", err)
	}
	if len(rows) == 0 {
		return nil, os.ErrNotExist
	}
	row := rows[0]

	env := row.Config.Env
	if env == nil {
		env = []string{}
	}

	mounts := make([]MountInfo, 0, len(row.Mounts))
	for _, m := range row.Mounts {
		mounts = append(mounts, MountInfo{
			Type:        m.Type,
			Source:      m.Source,
			Destination: m.Destination,
			RW:          m.RW,
		})
	}

	networks := make([]NetworkAttachment, 0, len(row.NetworkSettings.Networks))
	for name, netInfo := range row.NetworkSettings.Networks {
		networks = append(networks, NetworkAttachment{
			Name:      name,
			IPAddress: netInfo.IPAddress,
		})
	}
	sort.Slice(networks, func(i, j int) bool {
		return networks[i].Name < networks[j].Name
	})

	ports := []string{}
	for containerPort, bindings := range row.NetworkSettings.Ports {
		if len(bindings) == 0 {
			ports = append(ports, containerPort)
			continue
		}
		for _, b := range bindings {
			host := b.HostPort
			if b.HostIP != "" {
				host = b.HostIP + ":" + b.HostPort
			}
			ports = append(ports, host+"->"+containerPort)
		}
	}
	sort.Strings(ports)

	return &ContainerInspect{
		ID:            row.ID,
		Name:          strings.TrimPrefix(row.Name, "/"),
		Image:         row.Config.Image,
		State:         row.State.Status,
		RestartPolicy: row.HostConfig.RestartPolicy.Name,
		Env:           env,
		Mounts:        mounts,
		Networks:      networks,
		Ports:         ports,
	}, nil
}

func ParseImagesNDJSON(raw string, usedRefs map[string]bool) []ImageInfo {
	out := []ImageInfo{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			ID           string `json:"ID"`
			Repository   string `json:"Repository"`
			Tag          string `json:"Tag"`
			Size         string `json:"Size"`
			CreatedSince string `json:"CreatedSince"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		inUse := false
		if usedRefs != nil {
			if usedRefs[row.ID] {
				inUse = true
			}
			shortID := strings.TrimPrefix(row.ID, "sha256:")
			if len(shortID) >= 12 && usedRefs[shortID[:12]] {
				inUse = true
			}
			if row.Repository != "" && row.Repository != "<none>" {
				if row.Tag != "" && row.Tag != "<none>" {
					if usedRefs[row.Repository+":"+row.Tag] {
						inUse = true
					}
				}
				if usedRefs[row.Repository] {
					inUse = true
				}
			}
		}
		out = append(out, ImageInfo{
			ID:           row.ID,
			Repository:   row.Repository,
			Tag:          row.Tag,
			Size:         row.Size,
			CreatedSince: row.CreatedSince,
			InUse:        inUse,
		})
	}
	return out
}

func ParseComposePSJSON(raw string) (running int, total int) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, 0
	}
	type psRow struct {
		State string `json:"State"`
	}
	if strings.HasPrefix(trimmed, "[") {
		var rows []psRow
		if err := json.Unmarshal([]byte(trimmed), &rows); err == nil {
			for _, row := range rows {
				total++
				if strings.EqualFold(row.State, "running") {
					running++
				}
			}
			return running, total
		}
	}
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row psRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		total++
		if strings.EqualFold(row.State, "running") {
			running++
		}
	}
	return running, total
}

func ParseVolumesNDJSON(raw string) []VolumeInfo {
	out := []VolumeInfo{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			Name       string `json:"Name"`
			Driver     string `json:"Driver"`
			Mountpoint string `json:"Mountpoint"`
			Scope      string `json:"Scope"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		out = append(out, VolumeInfo{
			Name:       row.Name,
			Driver:     row.Driver,
			Mountpoint: row.Mountpoint,
			Scope:      row.Scope,
		})
	}
	return out
}

func ParseNetworksNDJSON(raw string) []NetworkInfo {
	out := []NetworkInfo{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row struct {
			ID     string `json:"ID"`
			Name   string `json:"Name"`
			Driver string `json:"Driver"`
			Scope  string `json:"Scope"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			continue
		}
		out = append(out, NetworkInfo{
			ID:     row.ID,
			Name:   row.Name,
			Driver: row.Driver,
			Scope:  row.Scope,
		})
	}
	return out
}

func (s *Service) Overview(ctx context.Context) Overview {
	ov := Overview{
		DiskUsage: []DiskUsageItem{},
	}
	verOut, err := runDockerStdout(ctx, 5*time.Second, "", "info", "--format", "{{.ServerVersion}}")
	if err != nil {
		ov.Available = false
		ov.Error = err.Error()
		return ov
	}
	ov.Available = true
	ov.ServerVersion = strings.TrimSpace(verOut)

	if statesOut, err := runDockerStdout(ctx, 5*time.Second, "", "ps", "-a", "--format", "{{.State}}"); err == nil {
		for _, line := range strings.Split(statesOut, "\n") {
			st := strings.TrimSpace(line)
			if st == "" {
				continue
			}
			if st == "running" {
				ov.ContainersRunning++
			} else {
				ov.ContainersStopped++
			}
		}
	}
	if imgOut, err := runDockerStdout(ctx, 5*time.Second, "", "images", "-q"); err == nil {
		ov.ImagesCount = countNonEmptyLines(imgOut)
	}
	if volOut, err := runDockerStdout(ctx, 5*time.Second, "", "volume", "ls", "-q"); err == nil {
		ov.VolumesCount = countNonEmptyLines(volOut)
	}
	if netOut, err := runDockerStdout(ctx, 5*time.Second, "", "network", "ls", "-q"); err == nil {
		ov.NetworksCount = countNonEmptyLines(netOut)
	}
	if dfOut, err := runDockerStdout(ctx, 10*time.Second, "", "system", "df", "--format", "{{json .}}"); err == nil {
		ov.DiskUsage = ParseDiskUsageNDJSON(dfOut)
	}
	return ov
}

func (s *Service) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	psOut, err := runDockerStdout(ctx, 10*time.Second, "", "ps", "-a", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	containers := ParseContainersNDJSON(psOut)
	hasRunning := false
	for _, c := range containers {
		if c.State == "running" {
			hasRunning = true
			break
		}
	}
	if hasRunning {
		if statsOut, stErr := runDockerStdout(ctx, 10*time.Second, "", "stats", "--no-stream", "--format", "{{json .}}"); stErr == nil {
			MergeContainerStats(containers, ParseStatsNDJSON(statsOut))
		}
	}
	return containers, nil
}

func (s *Service) ContainerAction(ctx context.Context, id, action string) error {
	if !ValidateContainerRef(id) {
		return ErrInvalidRef
	}
	switch action {
	case "start", "stop", "restart":
	default:
		return ErrInvalidAction
	}
	_, err := runDockerCombined(ctx, 30*time.Second, "", action, id)
	return err
}

func (s *Service) RemoveContainer(ctx context.Context, id string, force bool) error {
	if !ValidateContainerRef(id) {
		return ErrInvalidRef
	}
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, id)
	_, err := runDockerCombined(ctx, 30*time.Second, "", args...)
	return err
}

func (s *Service) ContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	if !ValidateContainerRef(id) {
		return "", ErrInvalidRef
	}
	if tail <= 0 {
		tail = 300
	}
	if tail < 10 {
		tail = 10
	}
	if tail > 2000 {
		tail = 2000
	}
	return runDockerCombined(ctx, 15*time.Second, "", "logs", "--tail", fmt.Sprintf("%d", tail), id)
}

func (s *Service) InspectContainer(ctx context.Context, id string) (*ContainerInspect, error) {
	if !ValidateContainerRef(id) {
		return nil, ErrInvalidRef
	}
	out, err := runDockerStdout(ctx, 10*time.Second, "", "inspect", id)
	if err != nil {
		return nil, err
	}
	return ParseContainerInspect(out)
}

func (s *Service) ListImages(ctx context.Context) ([]ImageInfo, error) {
	imgOut, err := runDockerStdout(ctx, 10*time.Second, "", "images", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	usedRefs := make(map[string]bool)
	if psOut, psErr := runDockerStdout(ctx, 5*time.Second, "", "ps", "-a", "--no-trunc", "--format", "{{.ID}}\t{{.Image}}"); psErr == nil {
		containerIDs := []string{}
		for _, line := range strings.Split(psOut, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) == 2 {
				containerIDs = append(containerIDs, parts[0])
				usedRefs[parts[1]] = true
			}
		}
		if len(containerIDs) > 0 {
			inspectArgs := append([]string{"inspect", "--format", "{{.Image}}"}, containerIDs...)
			if inspOut, inspErr := runDockerStdout(ctx, 5*time.Second, "", inspectArgs...); inspErr == nil {
				for _, line := range strings.Split(inspOut, "\n") {
					ref := strings.TrimSpace(line)
					if ref != "" {
						usedRefs[ref] = true
					}
				}
			}
		}
	}
	return ParseImagesNDJSON(imgOut, usedRefs), nil
}

func (s *Service) PullImage(ctx context.Context, image string) (string, error) {
	if !ValidateImageRef(image) {
		return "", ErrInvalidRef
	}
	return runDockerCombined(ctx, 5*time.Minute, "", "pull", image)
}

func (s *Service) RemoveImage(ctx context.Context, id string, force bool) error {
	if !ValidateImageRef(id) {
		return ErrInvalidRef
	}
	args := []string{"rmi"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, id)
	_, err := runDockerCombined(ctx, 30*time.Second, "", args...)
	return err
}

func (s *Service) PruneImages(ctx context.Context) (string, error) {
	return runDockerCombined(ctx, 60*time.Second, "", "image", "prune", "-f")
}

func (s *Service) stackComposePath(name string) (stackDir, composePath string, err error) {
	if !ValidateStackName(name) {
		return "", "", ErrInvalidRef
	}
	stackDir = filepath.Join(s.stacksDir, name)
	composePath = filepath.Join(stackDir, "docker-compose.yml")
	return stackDir, composePath, nil
}

func (s *Service) ListStacks(ctx context.Context) ([]StackSummary, error) {
	entries, err := os.ReadDir(s.stacksDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []StackSummary{}, nil
		}
		return nil, err
	}
	stacks := []StackSummary{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !ValidateStackName(name) {
			continue
		}
		stackDir := filepath.Join(s.stacksDir, name)
		composePath := filepath.Join(stackDir, "docker-compose.yml")
		info, statErr := os.Stat(composePath)
		if statErr != nil || info.IsDir() {
			continue
		}

		running, total := 0, 0
		if psOut, psErr := runDockerStdout(ctx, 5*time.Second, stackDir, "compose", "-p", "ps-stack-"+name, "-f", "docker-compose.yml", "ps", "--format", "json"); psErr == nil {
			running, total = ParseComposePSJSON(psOut)
		}
		status := "stopped"
		if total > 0 && running == total {
			status = "running"
		} else if running > 0 {
			status = "partial"
		}
		stacks = append(stacks, StackSummary{
			Name:         name,
			Status:       status,
			RunningCount: running,
			TotalCount:   total,
			UpdatedAt:    info.ModTime().Unix(),
		})
	}
	sort.Slice(stacks, func(i, j int) bool {
		return stacks[i].Name < stacks[j].Name
	})
	return stacks, nil
}

func (s *Service) GetStack(name string) (string, error) {
	_, composePath, err := s.stackComposePath(name)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(composePath)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (s *Service) SaveStack(name, composeYaml string) error {
	stackDir, composePath, err := s.stackComposePath(name)
	if err != nil {
		return err
	}
	if strings.TrimSpace(composeYaml) == "" {
		return ErrInvalidYAML
	}
	var parsed any
	if err := yaml.Unmarshal([]byte(composeYaml), &parsed); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidYAML, err)
	}
	if err := os.MkdirAll(stackDir, 0o755); err != nil {
		return err
	}
	return files.AtomicWriteFile(composePath, []byte(composeYaml), 0o644)
}

func (s *Service) StackAction(ctx context.Context, name, action string) (string, error) {
	stackDir, composePath, err := s.stackComposePath(name)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(composePath); err != nil {
		return "", err
	}
	project := "ps-stack-" + name
	switch action {
	case "up":
		return runDockerCombined(ctx, 5*time.Minute, stackDir, "compose", "-p", project, "-f", "docker-compose.yml", "up", "-d")
	case "down":
		return runDockerCombined(ctx, 2*time.Minute, stackDir, "compose", "-p", project, "-f", "docker-compose.yml", "down")
	case "recreate":
		pullOut, err := runDockerCombined(ctx, 5*time.Minute, stackDir, "compose", "-p", project, "-f", "docker-compose.yml", "pull")
		if err != nil {
			return pullOut, err
		}
		upOut, err := runDockerCombined(ctx, 5*time.Minute, stackDir, "compose", "-p", project, "-f", "docker-compose.yml", "up", "-d", "--force-recreate")
		return strings.TrimSpace(pullOut + "\n" + upOut), err
	default:
		return "", ErrInvalidAction
	}
}

func (s *Service) StackLogs(ctx context.Context, name string) (string, error) {
	stackDir, composePath, err := s.stackComposePath(name)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(composePath); err != nil {
		return "", err
	}
	return runDockerCombined(ctx, 15*time.Second, stackDir, "compose", "-p", "ps-stack-"+name, "-f", "docker-compose.yml", "logs", "--no-color", "--tail=200")
}

func (s *Service) DeleteStack(ctx context.Context, name string) error {
	stackDir, composePath, err := s.stackComposePath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(stackDir); err != nil {
		return err
	}
	if _, statErr := os.Stat(composePath); statErr == nil {
		_, _ = runDockerCombined(ctx, 2*time.Minute, stackDir, "compose", "-p", "ps-stack-"+name, "-f", "docker-compose.yml", "down", "--remove-orphans")
	}
	return os.RemoveAll(stackDir)
}

func (s *Service) ListVolumes(ctx context.Context) ([]VolumeInfo, error) {
	out, err := runDockerStdout(ctx, 10*time.Second, "", "volume", "ls", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	return ParseVolumesNDJSON(out), nil
}

func (s *Service) RemoveVolume(ctx context.Context, name string) error {
	if !ValidateVolumeOrNetwork(name) {
		return ErrInvalidRef
	}
	_, err := runDockerCombined(ctx, 30*time.Second, "", "volume", "rm", name)
	return err
}

func (s *Service) ListNetworks(ctx context.Context) ([]NetworkInfo, error) {
	out, err := runDockerStdout(ctx, 10*time.Second, "", "network", "ls", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	return ParseNetworksNDJSON(out), nil
}

func (s *Service) RemoveNetwork(ctx context.Context, id string) error {
	if !ValidateVolumeOrNetwork(id) {
		return ErrInvalidRef
	}
	switch strings.ToLower(id) {
	case "bridge", "host", "none":
		return ErrProtectedNetwork
	}
	_, err := runDockerCombined(ctx, 30*time.Second, "", "network", "rm", id)
	return err
}
