package wargame

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"personal-server/internal/config"
	"personal-server/internal/files"
)

const (
	wargameNetworkName = "ps-wargame-net"
	maxLogReadBytes    = 256 * 1024
)

var (
	ErrNotFound          = errors.New("인스턴스를 찾을 수 없습니다")
	ErrInvalidState      = errors.New("현재 인스턴스 상태에서는 수행할 수 없는 작업입니다")
	ErrBadRequest        = errors.New("잘못된 요청 값입니다")
	ErrDockerUnavailable = errors.New("Docker 데몬을 사용할 수 없습니다")
)

type WargameInstance struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Mode           string        `json:"mode"`
	TemplateKind   string        `json:"templateKind"`
	EntrypointFile string        `json:"entrypointFile"`
	ComposeFile    string        `json:"composeFile"`
	RelaxSecurity  bool          `json:"relaxSecurity"`
	Status         string        `json:"status"`
	BindIP         string        `json:"bindIp"`
	Ports          []PortBinding `json:"ports"`
	Warnings       []string      `json:"warnings"`
	ExpiresAt      int64         `json:"expiresAt"`
	CreatedAt      int64         `json:"createdAt"`
	LastError      string        `json:"lastError"`

	workDir     string `json:"-"`
	imageTag    string `json:"-"`
	containerID string `json:"-"`
}

type StartOptions struct {
	Name          *string `json:"name"`
	TemplateKind  *string `json:"templateKind"`
	ContainerPort *int    `json:"containerPort"`
	ConnKind      *string `json:"connKind"`
	RelaxSecurity *bool   `json:"relaxSecurity"`
}

type buildCancelEntry struct {
	gen    uint64
	cancel context.CancelFunc
}

type Service struct {
	db    *sql.DB
	jail  *files.Jail
	cfg   config.WargameConfig
	ports *PortAllocator

	locksMu       sync.Mutex
	instanceLocks map[string]*sync.Mutex

	cancelsMu    sync.Mutex
	nextBuildGen uint64
	buildCancels map[string]buildCancelEntry
}

func NewService(conn *sql.DB, jail *files.Jail, cfg config.WargameConfig) (*Service, error) {
	if cfg.BindIP == "" {
		cfg.BindIP = "127.0.0.1"
	}
	if cfg.PortStart <= 0 {
		cfg.PortStart = 30000
	}
	if cfg.PortEnd <= cfg.PortStart {
		cfg.PortEnd = cfg.PortStart + 9999
	}
	if cfg.DefaultTTLHours <= 0 {
		cfg.DefaultTTLHours = 3
	}
	if cfg.MaxArchiveBytes <= 0 {
		cfg.MaxArchiveBytes = 1024 * 1024 * 1024
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = "./data/wargames"
	}
	if err := os.MkdirAll(cfg.WorkDir, 0o755); err != nil {
		return nil, fmt.Errorf("create wargame work dir: %w", err)
	}
	if abs, err := filepath.Abs(cfg.WorkDir); err == nil {
		if realDir, evalErr := filepath.EvalSymlinks(abs); evalErr == nil {
			cfg.WorkDir = realDir
		} else {
			cfg.WorkDir = abs
		}
	}

	svc := &Service{
		db:            conn,
		jail:          jail,
		cfg:           cfg,
		ports:         NewPortAllocator(cfg.BindIP, cfg.PortStart, cfg.PortEnd),
		instanceLocks: make(map[string]*sync.Mutex),
		buildCancels:  make(map[string]buildCancelEntry),
	}

	if err := svc.reconcileOnStartup(); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *Service) reconcileOnStartup() error {
	if _, err := s.db.Exec(
		`UPDATE wargame_instances SET status = 'failed', last_error = ? WHERE status = 'building'`,
		"서버 재시작으로 빌드가 중단되었습니다",
	); err != nil {
		return fmt.Errorf("reconcile building wargames: %w", err)
	}

	instances, err := s.ListInstances()
	if err != nil {
		return fmt.Errorf("load wargame instances: %w", err)
	}

	probeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	dockerOK, _ := s.DockerStatus(probeCtx)

	for _, inst := range instances {
		if inst.Status != "running" {
			continue
		}
		if dockerOK && !s.isInstanceContainerRunning(probeCtx, inst) {
			_, _ = s.db.Exec(`UPDATE wargame_instances SET status = 'stopped' WHERE id = ?`, inst.ID)
			continue
		}
		s.ports.MarkOwned(inst.ID, inst.Ports)
	}
	return nil
}

func (s *Service) isInstanceContainerRunning(ctx context.Context, inst WargameInstance) bool {
	if inst.Mode == "compose" {
		composePath := filepath.Join(inst.workDir, "src", "compose.ps.yaml")
		out, err := runCmdCombined(ctx, filepath.Join(inst.workDir, "src"),
			"docker", "compose", "-p", "ps-wg-"+inst.ID, "-f", composePath, "ps", "--status", "running", "-q",
		)
		return err == nil && strings.TrimSpace(out) != ""
	}
	out, err := runCmdCombined(ctx, "", "docker", "inspect", "-f", "{{.State.Running}}", "ps-wg-"+inst.ID)
	return err == nil && strings.TrimSpace(out) == "true"
}

func (s *Service) StartBackground(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		s.reapExpired(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.reapExpired(ctx)
			}
		}
	}()
}

func (s *Service) reapExpired(ctx context.Context) {
	now := time.Now().Unix()
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM wargame_instances WHERE expires_at > 0 AND expires_at <= ?`, now,
	)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr == nil {
			ids = append(ids, id)
		}
	}
	_ = rows.Close()

	for _, id := range ids {
		reapCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_ = s.Terminate(reapCtx, id)
		cancel()
	}
}

func (s *Service) DockerStatus(ctx context.Context) (bool, string) {
	if _, err := exec.LookPath("docker"); err != nil {
		return false, "호스트에서 docker 실행 파일을 찾을 수 없습니다."
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()

	out, err := runCmdCombined(probeCtx, "", "docker", "info", "--format", "{{.ServerVersion}}")
	if err != nil {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = err.Error()
		}
		if idx := strings.IndexByte(msg, '\n'); idx != -1 {
			msg = strings.TrimSpace(msg[:idx])
		}
		return false, "Docker 데몬에 연결할 수 없습니다: " + msg
	}
	return true, ""
}

func (s *Service) lockFor(id string) *sync.Mutex {
	s.locksMu.Lock()
	defer s.locksMu.Unlock()
	mu, ok := s.instanceLocks[id]
	if !ok {
		mu = &sync.Mutex{}
		s.instanceLocks[id] = mu
	}
	return mu
}

func (s *Service) registerBuildCancel(id string, cancel context.CancelFunc) uint64 {
	s.cancelsMu.Lock()
	defer s.cancelsMu.Unlock()
	if prev, ok := s.buildCancels[id]; ok {
		prev.cancel()
	}
	s.nextBuildGen++
	gen := s.nextBuildGen
	s.buildCancels[id] = buildCancelEntry{gen: gen, cancel: cancel}
	return gen
}

func (s *Service) clearBuildCancel(id string, gen uint64) {
	s.cancelsMu.Lock()
	defer s.cancelsMu.Unlock()
	if cur, ok := s.buildCancels[id]; ok && cur.gen == gen {
		delete(s.buildCancels, id)
	}
}

func (s *Service) cancelBuild(id string) {
	s.cancelsMu.Lock()
	entry, ok := s.buildCancels[id]
	if ok {
		delete(s.buildCancels, id)
	}
	s.cancelsMu.Unlock()
	if ok {
		entry.cancel()
	}
}

func isValidInstanceID(id string) bool {
	if len(id) != 12 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func (s *Service) generateUniqueID() (string, error) {
	for i := 0; i < 8; i++ {
		raw := make([]byte, 6)
		if _, err := rand.Read(raw); err != nil {
			return "", err
		}
		id := hex.EncodeToString(raw)
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM wargame_instances WHERE id = ?`, id).Scan(&count); err == nil && count == 0 {
			return id, nil
		}
	}
	return "", errors.New("고유 인스턴스 ID 생성에 실패했습니다")
}

func (s *Service) PrepareFromArchive(ctx context.Context, archivePath, origFilename, displayName string) (WargameInstance, error) {
	if !IsSupportedArchive(origFilename) {
		return WargameInstance{}, ErrUnsupportedArchive
	}
	id, err := s.generateUniqueID()
	if err != nil {
		return WargameInstance{}, err
	}
	instWorkDir := filepath.Join(s.cfg.WorkDir, id)
	srcDir := filepath.Join(instWorkDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return WargameInstance{}, err
	}

	cleanupOnErr := true
	defer func() {
		if cleanupOnErr {
			_ = os.RemoveAll(instWorkDir)
		}
	}()

	// Copy or link archive name suffix if needed so ExtractArchive sees the right extension.
	if err := ExtractArchive(ctx, archivePath, srcDir, s.cfg.MaxArchiveBytes); err != nil {
		return WargameInstance{}, err
	}

	name := strings.TrimSpace(displayName)
	if name == "" {
		name = StripArchiveExt(origFilename)
	}
	if name == "" {
		name = "wargame-" + id[:6]
	}

	inst, err := s.finalizePreparedInstance(id, name, instWorkDir, srcDir)
	if err != nil {
		return WargameInstance{}, err
	}
	cleanupOnErr = false
	return inst, nil
}

func (s *Service) PrepareFromMount(ctx context.Context, rootID, relPath, displayName string) (WargameInstance, error) {
	if s.jail == nil {
		return WargameInstance{}, errors.New("파일 마운트 시스템이 초기화되지 않았습니다")
	}
	targetReal, cleanRel, err := s.jail.ResolveExistingForTier(rootID, relPath, "private")
	if err != nil {
		return WargameInstance{}, err
	}
	rootCfg, ok := s.jail.Root(rootID)
	if !ok {
		return WargameInstance{}, files.ErrUnknownRoot
	}
	st, err := os.Stat(targetReal)
	if err != nil {
		return WargameInstance{}, err
	}

	id, err := s.generateUniqueID()
	if err != nil {
		return WargameInstance{}, err
	}
	instWorkDir := filepath.Join(s.cfg.WorkDir, id)
	srcDir := filepath.Join(instWorkDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return WargameInstance{}, err
	}

	cleanupOnErr := true
	defer func() {
		if cleanupOnErr {
			_ = os.RemoveAll(instWorkDir)
		}
	}()

	switch {
	case st.IsDir():
		if err := CopyDirSafe(targetReal, srcDir, rootCfg.Path, s.cfg.MaxArchiveBytes); err != nil {
			return WargameInstance{}, err
		}
	case st.Mode().IsRegular() && IsSupportedArchive(targetReal):
		if err := ExtractArchive(ctx, targetReal, srcDir, s.cfg.MaxArchiveBytes); err != nil {
			return WargameInstance{}, err
		}
	case st.Mode().IsRegular():
		if st.Size() > s.cfg.MaxArchiveBytes {
			return WargameInstance{}, ErrArchiveTooLarge
		}
		in, openErr := os.Open(targetReal)
		if openErr != nil {
			return WargameInstance{}, openErr
		}
		dstFile := filepath.Join(srcDir, filepath.Base(targetReal))
		_, copyErr := writeBoundedFile(dstFile, in, filePermFromArchive(st.Mode()), s.cfg.MaxArchiveBytes)
		_ = in.Close()
		if copyErr != nil {
			return WargameInstance{}, copyErr
		}
	default:
		return WargameInstance{}, errors.New("유효한 디렉토리 또는 아카이브 파일이 아닙니다")
	}

	name := strings.TrimSpace(displayName)
	if name == "" {
		if cleanRel == "" {
			name = rootCfg.Name
		} else {
			name = StripArchiveExt(filepath.Base(targetReal))
		}
	}
	if name == "" {
		name = "wargame-" + id[:6]
	}

	inst, err := s.finalizePreparedInstance(id, name, instWorkDir, srcDir)
	if err != nil {
		return WargameInstance{}, err
	}
	cleanupOnErr = false
	return inst, nil
}

func (s *Service) finalizePreparedInstance(id, name, instWorkDir, srcDir string) (WargameInstance, error) {
	det, err := Detect(srcDir)
	if err != nil {
		return WargameInstance{}, err
	}
	now := time.Now().Unix()
	expiresAt := now + int64(s.cfg.DefaultTTLHours)*3600

	inst := WargameInstance{
		ID:             id,
		Name:           name,
		Mode:           det.Mode,
		TemplateKind:   det.TemplateKind,
		EntrypointFile: det.EntrypointFile,
		ComposeFile:    det.ComposeFile,
		RelaxSecurity:  false,
		Status:         "detected",
		BindIP:         s.cfg.BindIP,
		Ports:          nonNilPorts(det.Ports),
		Warnings:       nonNilStrings(det.Warnings),
		ExpiresAt:      expiresAt,
		CreatedAt:      now,
		LastError:      "",
		workDir:        instWorkDir,
	}

	portsJSON, _ := json.Marshal(inst.Ports)
	warningsJSON, _ := json.Marshal(inst.Warnings)

	if _, err := s.db.Exec(
		`INSERT INTO wargame_instances (
			id, name, mode, template_kind, entrypoint_file, compose_file, relax_security,
			status, work_dir, image_tag, container_id, ports_json, warnings_json,
			expires_at, created_at, last_error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inst.ID, inst.Name, inst.Mode, inst.TemplateKind, inst.EntrypointFile, inst.ComposeFile,
		boolToInt(inst.RelaxSecurity), inst.Status, inst.workDir, inst.imageTag, inst.containerID,
		string(portsJSON), string(warningsJSON), inst.ExpiresAt, inst.CreatedAt, inst.LastError,
	); err != nil {
		return WargameInstance{}, fmt.Errorf("insert wargame instance: %w", err)
	}
	return inst, nil
}

func (s *Service) ListInstances() ([]WargameInstance, error) {
	rows, err := s.db.Query(
		`SELECT id, name, mode, template_kind, entrypoint_file, compose_file, relax_security,
		        status, work_dir, image_tag, container_id, ports_json, warnings_json,
		        expires_at, created_at, last_error
		 FROM wargame_instances
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]WargameInstance, 0)
	for rows.Next() {
		inst, err := s.scanInstance(rows)
		if err != nil {
			continue
		}
		out = append(out, inst)
	}
	return out, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Service) scanInstance(rs rowScanner) (WargameInstance, error) {
	var inst WargameInstance
	var relaxInt int
	var portsRaw, warningsRaw string
	if err := rs.Scan(
		&inst.ID, &inst.Name, &inst.Mode, &inst.TemplateKind, &inst.EntrypointFile, &inst.ComposeFile, &relaxInt,
		&inst.Status, &inst.workDir, &inst.imageTag, &inst.containerID, &portsRaw, &warningsRaw,
		&inst.ExpiresAt, &inst.CreatedAt, &inst.LastError,
	); err != nil {
		return WargameInstance{}, err
	}
	inst.RelaxSecurity = relaxInt == 1
	inst.BindIP = s.cfg.BindIP
	_ = json.Unmarshal([]byte(portsRaw), &inst.Ports)
	_ = json.Unmarshal([]byte(warningsRaw), &inst.Warnings)
	inst.Ports = nonNilPorts(inst.Ports)
	inst.Warnings = nonNilStrings(inst.Warnings)
	return inst, nil
}

func (s *Service) GetInstance(id string) (WargameInstance, error) {
	if !isValidInstanceID(id) {
		return WargameInstance{}, ErrNotFound
	}
	row := s.db.QueryRow(
		`SELECT id, name, mode, template_kind, entrypoint_file, compose_file, relax_security,
		        status, work_dir, image_tag, container_id, ports_json, warnings_json,
		        expires_at, created_at, last_error
		 FROM wargame_instances WHERE id = ?`, id,
	)
	inst, err := s.scanInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return WargameInstance{}, ErrNotFound
	}
	return inst, err
}

func (s *Service) saveInstance(inst WargameInstance) error {
	portsJSON, _ := json.Marshal(nonNilPorts(inst.Ports))
	warningsJSON, _ := json.Marshal(nonNilStrings(inst.Warnings))
	_, err := s.db.Exec(
		`UPDATE wargame_instances
		 SET name = ?, mode = ?, template_kind = ?, entrypoint_file = ?, compose_file = ?,
		     relax_security = ?, status = ?, image_tag = ?, container_id = ?,
		     ports_json = ?, warnings_json = ?, expires_at = ?, last_error = ?
		 WHERE id = ?`,
		inst.Name, inst.Mode, inst.TemplateKind, inst.EntrypointFile, inst.ComposeFile,
		boolToInt(inst.RelaxSecurity), inst.Status, inst.imageTag, inst.containerID,
		string(portsJSON), string(warningsJSON), inst.ExpiresAt, inst.LastError,
		inst.ID,
	)
	return err
}

func (s *Service) Start(ctx context.Context, id string, opts StartOptions) (WargameInstance, error) {
	if !isValidInstanceID(id) {
		return WargameInstance{}, ErrNotFound
	}
	if ok, msg := s.DockerStatus(ctx); !ok {
		return WargameInstance{}, fmt.Errorf("%w: %s", ErrDockerUnavailable, msg)
	}

	mu := s.lockFor(id)
	mu.Lock()
	defer mu.Unlock()

	inst, err := s.GetInstance(id)
	if err != nil {
		return WargameInstance{}, err
	}
	if inst.Status == "building" || inst.Status == "running" {
		return WargameInstance{}, fmt.Errorf("%w: 이미 %s 상태입니다", ErrInvalidState, inst.Status)
	}

	if opts.Name != nil {
		if trimmed := strings.TrimSpace(*opts.Name); trimmed != "" {
			inst.Name = trimmed
		}
	}
	if opts.RelaxSecurity != nil {
		inst.RelaxSecurity = *opts.RelaxSecurity
	}
	if opts.TemplateKind != nil && strings.TrimSpace(*opts.TemplateKind) != "" {
		tk := strings.TrimSpace(*opts.TemplateKind)
		if !IsValidTemplateKind(tk) {
			return WargameInstance{}, fmt.Errorf("%w: 지원하지 않는 템플릿(%s)입니다", ErrBadRequest, tk)
		}
		inst.TemplateKind = tk
	}
	if opts.ContainerPort != nil && *opts.ContainerPort > 0 {
		cp := *opts.ContainerPort
		if cp > 65535 {
			return WargameInstance{}, fmt.Errorf("%w: 유효하지 않은 포트 번호입니다", ErrBadRequest)
		}
		if len(inst.Ports) == 0 {
			inst.Ports = []PortBinding{{
				Service:       "default",
				ContainerPort: cp,
				Protocol:      "tcp",
				ConnKind:      "nc",
			}}
		} else {
			inst.Ports[0].ContainerPort = cp
		}
	}
	if opts.ConnKind != nil && strings.TrimSpace(*opts.ConnKind) != "" {
		ck := strings.TrimSpace(*opts.ConnKind)
		if ck != "nc" && ck != "http" {
			return WargameInstance{}, fmt.Errorf("%w: connKind는 nc 또는 http여야 합니다", ErrBadRequest)
		}
		if len(inst.Ports) > 0 {
			inst.Ports[0].ConnKind = ck
		}
	}

	reservedPorts, err := s.ports.ReserveForInstance(inst.ID, inst.Ports)
	if err != nil {
		return WargameInstance{}, err
	}
	inst.Ports = reservedPorts

	minExp := time.Now().Unix() + int64(s.cfg.DefaultTTLHours)*3600
	if inst.ExpiresAt < minExp {
		inst.ExpiresAt = minExp
	}
	inst.Status = "building"
	inst.LastError = ""

	if err := s.saveInstance(inst); err != nil {
		s.ports.ReleaseInstance(inst.ID)
		return WargameInstance{}, err
	}

	buildLogPath := filepath.Join(inst.workDir, "build.log")
	_ = os.WriteFile(buildLogPath, nil, 0o644)

	buildCtx, cancel := context.WithCancel(context.Background())
	gen := s.registerBuildCancel(inst.ID, cancel)

	go s.executeBuildAndRun(buildCtx, gen, inst)
	return inst, nil
}

func (s *Service) executeBuildAndRun(ctx context.Context, gen uint64, inst WargameInstance) {
	defer s.clearBuildCancel(inst.ID, gen)

	srcDir := filepath.Join(inst.workDir, "src")
	logPath := filepath.Join(inst.workDir, "build.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		s.finishBuildResult(ctx, inst.ID, "", "", nil, err)
		return
	}
	defer func() { _ = logFile.Close() }()

	writeStep := func(line string) {
		_, _ = fmt.Fprintf(logFile, "==> %s\n", line)
	}

	var extraWarnings []string
	var imageTag, containerID string

	switch inst.Mode {
	case "compose":
		writeStep("Compose 설정 보안 검증 및 포트 바인딩 재작성 중...")
		w, sanErr := WriteSanitizedCompose(srcDir, inst.ComposeFile, s.cfg.BindIP, inst.Ports, inst.RelaxSecurity)
		if sanErr != nil {
			_, _ = fmt.Fprintf(logFile, "오류: %v\n", sanErr)
			s.finishBuildResult(ctx, inst.ID, "", "", nil, sanErr)
			return
		}
		extraWarnings = w
		for _, warn := range extraWarnings {
			_, _ = fmt.Fprintf(logFile, "[경고] %s\n", warn)
		}

		writeStep("docker compose up -d --build 실행 중...")
		if runErr := runCmdStreaming(ctx, srcDir, logFile,
			"docker", "compose", "-p", "ps-wg-"+inst.ID, "-f", "compose.ps.yaml", "up", "-d", "--build", "--force-recreate",
		); runErr != nil {
			s.finishBuildResult(ctx, inst.ID, "", "", extraWarnings, runErr)
			return
		}
		containerID = "compose:ps-wg-" + inst.ID

	case "template", "dockerfile":
		dockerfileName := "Dockerfile"
		if inst.Mode == "template" {
			targetPort := 9000
			if len(inst.Ports) > 0 && inst.Ports[0].ContainerPort > 0 {
				targetPort = inst.Ports[0].ContainerPort
			}
			writeStep(fmt.Sprintf("템플릿(%s) Dockerfile.ps 생성 중...", inst.TemplateKind))
			if tplErr := WriteTemplateDockerfile(srcDir, inst.TemplateKind, inst.EntrypointFile, targetPort); tplErr != nil {
				_, _ = fmt.Fprintf(logFile, "오류: %v\n", tplErr)
				s.finishBuildResult(ctx, inst.ID, "", "", nil, tplErr)
				return
			}
			dockerfileName = "Dockerfile.ps"
		}

		if netErr := ensureWargameNetwork(ctx, logFile); netErr != nil {
			s.finishBuildResult(ctx, inst.ID, "", "", nil, netErr)
			return
		}

		imageTag = "ps-wg-" + inst.ID + ":latest"
		writeStep(fmt.Sprintf("docker build (%s -> %s) 실행 중...", dockerfileName, imageTag))
		if buildErr := runCmdStreaming(ctx, srcDir, logFile,
			"docker", "build", "-t", imageTag, "-f", dockerfileName, ".",
		); buildErr != nil {
			s.finishBuildResult(ctx, inst.ID, "", "", nil, buildErr)
			return
		}

		_ = runCmdQuiet(ctx, "", "docker", "rm", "-f", "ps-wg-"+inst.ID)

		runArgs := buildStandaloneRunArgs(inst, s.cfg.BindIP)
		writeStep("컨테이너 시작 중...")
		out, runErr := runCmdCombined(ctx, srcDir, "docker", runArgs...)
		if out != "" {
			_, _ = logFile.WriteString(out + "\n")
		}
		if runErr != nil {
			s.finishBuildResult(ctx, inst.ID, imageTag, "", nil, fmt.Errorf("컨테이너 실행 실패: %s", firstLineOrErr(out, runErr)))
			return
		}
		containerID = strings.TrimSpace(out)

	default:
		s.finishBuildResult(ctx, inst.ID, "", "", nil, fmt.Errorf("알 수 없는 실행 모드: %s", inst.Mode))
		return
	}

	writeStep("실행 완료.")
	s.finishBuildResult(ctx, inst.ID, imageTag, containerID, extraWarnings, nil)
}

func (s *Service) finishBuildResult(ctx context.Context, id, imageTag, containerID string, extraWarnings []string, buildErr error) {
	mu := s.lockFor(id)
	mu.Lock()
	defer mu.Unlock()

	cur, err := s.GetInstance(id)
	if err != nil {
		// Instance was deleted while building; clean up any container created right before cancellation.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = runCmdQuiet(cleanupCtx, "", "docker", "rm", "-f", "ps-wg-"+id)
		return
	}

	if ctx.Err() != nil || cur.Status != "building" {
		if cur.Status != "running" {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if cur.Mode == "compose" {
				srcDir := filepath.Join(cur.workDir, "src")
				_ = runCmdQuiet(cleanupCtx, srcDir, "docker", "compose", "-p", "ps-wg-"+id, "-f", "compose.ps.yaml", "stop")
			} else {
				_ = runCmdQuiet(cleanupCtx, "", "docker", "rm", "-f", "ps-wg-"+id)
			}
		}
		return
	}

	if len(extraWarnings) > 0 {
		cur.Warnings = mergeUniqueStrings(cur.Warnings, extraWarnings)
	}
	if imageTag != "" {
		cur.imageTag = imageTag
	}

	if buildErr != nil {
		s.ports.ReleaseInstance(id)
		cur.Status = "failed"
		cur.LastError = buildErr.Error()
		_ = s.saveInstance(cur)
		return
	}

	cur.Status = "running"
	cur.containerID = containerID
	cur.LastError = ""
	_ = s.saveInstance(cur)
}

func buildStandaloneRunArgs(inst WargameInstance, bindIP string) []string {
	if bindIP == "" {
		bindIP = "127.0.0.1"
	}
	args := []string{
		"run", "-d",
		"--name", "ps-wg-" + inst.ID,
		"--network", wargameNetworkName,
		"--label", "personal-server.wargame.id=" + inst.ID,
	}
	hasPrivilegedPort := false
	for _, pb := range inst.Ports {
		if pb.ContainerPort > 0 && pb.ContainerPort < 1024 {
			hasPrivilegedPort = true
		}
		if pb.HostPort > 0 && pb.ContainerPort > 0 {
			args = append(args, "-p", fmt.Sprintf("%s:%d:%d/tcp", bindIP, pb.HostPort, pb.ContainerPort))
		}
	}
	args = append(args,
		"--cpus=0.5",
		"--memory=256m",
		"--pids-limit=100",
	)
	if !inst.RelaxSecurity {
		args = append(args,
			"--cap-drop=ALL",
			"--security-opt=no-new-privileges:true",
		)
		if inst.TemplateKind == "php-apache" || hasPrivilegedPort || inst.Mode == "dockerfile" {
			args = append(args,
				"--cap-add=CHOWN",
				"--cap-add=SETUID",
				"--cap-add=SETGID",
				"--cap-add=NET_BIND_SERVICE",
				"--cap-add=DAC_OVERRIDE",
			)
		}
	}
	imageTag := inst.imageTag
	if imageTag == "" {
		imageTag = "ps-wg-" + inst.ID + ":latest"
	}
	args = append(args, imageTag)
	return args
}

func ensureWargameNetwork(ctx context.Context, logOut io.Writer) error {
	if err := runCmdQuiet(ctx, "", "docker", "network", "inspect", wargameNetworkName); err == nil {
		return nil
	}
	if logOut != nil {
		_, _ = fmt.Fprintf(logOut, "==> 격리 브릿지 네트워크(%s) 생성 중...\n", wargameNetworkName)
	}
	out, err := runCmdCombined(ctx, "",
		"docker", "network", "create",
		"--driver", "bridge",
		"--opt", "com.docker.network.bridge.enable_icc=false",
		wargameNetworkName,
	)
	if err != nil && !strings.Contains(strings.ToLower(out), "already exists") {
		return fmt.Errorf("격리 네트워크 생성 실패: %s", firstLineOrErr(out, err))
	}
	return nil
}

func (s *Service) Reset(ctx context.Context, id string) (WargameInstance, error) {
	if !isValidInstanceID(id) {
		return WargameInstance{}, ErrNotFound
	}
	if ok, msg := s.DockerStatus(ctx); !ok {
		return WargameInstance{}, fmt.Errorf("%w: %s", ErrDockerUnavailable, msg)
	}

	mu := s.lockFor(id)
	mu.Lock()
	defer mu.Unlock()

	inst, err := s.GetInstance(id)
	if err != nil {
		return WargameInstance{}, err
	}
	if inst.Status == "building" {
		return WargameInstance{}, fmt.Errorf("%w: 빌드 중에는 리셋할 수 없습니다", ErrInvalidState)
	}
	if inst.Status == "detected" || (inst.Mode != "compose" && inst.imageTag == "") {
		return WargameInstance{}, fmt.Errorf("%w: 빌드된 이미지가 없습니다. 먼저 시작(Start)을 실행하세요", ErrInvalidState)
	}

	reservedPorts, err := s.ports.ReserveForInstance(inst.ID, inst.Ports)
	if err != nil {
		return WargameInstance{}, err
	}
	inst.Ports = reservedPorts
	srcDir := filepath.Join(inst.workDir, "src")

	if inst.Mode == "compose" {
		if _, sanErr := WriteSanitizedCompose(srcDir, inst.ComposeFile, s.cfg.BindIP, inst.Ports, inst.RelaxSecurity); sanErr != nil {
			return WargameInstance{}, sanErr
		}
		out, runErr := runCmdCombined(ctx, srcDir,
			"docker", "compose", "-p", "ps-wg-"+inst.ID, "-f", "compose.ps.yaml", "up", "-d", "--force-recreate", "--no-build",
		)
		if runErr != nil {
			return WargameInstance{}, fmt.Errorf("Compose 리셋 실패: %s", firstLineOrErr(out, runErr))
		}
		inst.containerID = "compose:ps-wg-" + inst.ID
	} else {
		if netErr := ensureWargameNetwork(ctx, nil); netErr != nil {
			return WargameInstance{}, netErr
		}
		_ = runCmdQuiet(ctx, "", "docker", "rm", "-f", "ps-wg-"+inst.ID)
		out, runErr := runCmdCombined(ctx, srcDir, "docker", buildStandaloneRunArgs(inst, s.cfg.BindIP)...)
		if runErr != nil {
			return WargameInstance{}, fmt.Errorf("컨테이너 리셋 실패: %s", firstLineOrErr(out, runErr))
		}
		inst.containerID = strings.TrimSpace(out)
	}

	inst.Status = "running"
	inst.LastError = ""
	if err := s.saveInstance(inst); err != nil {
		return WargameInstance{}, err
	}
	return inst, nil
}

func (s *Service) Stop(ctx context.Context, id string) (WargameInstance, error) {
	if !isValidInstanceID(id) {
		return WargameInstance{}, ErrNotFound
	}
	s.cancelBuild(id)

	mu := s.lockFor(id)
	mu.Lock()
	defer mu.Unlock()

	inst, err := s.GetInstance(id)
	if err != nil {
		return WargameInstance{}, err
	}

	if _, lookErr := exec.LookPath("docker"); lookErr == nil {
		srcDir := filepath.Join(inst.workDir, "src")
		if inst.Mode == "compose" {
			composePS := filepath.Join(srcDir, "compose.ps.yaml")
			if _, statErr := os.Stat(composePS); statErr == nil {
				_ = runCmdQuiet(ctx, srcDir, "docker", "compose", "-p", "ps-wg-"+inst.ID, "-f", "compose.ps.yaml", "stop")
			}
		} else {
			_ = runCmdQuiet(ctx, "", "docker", "rm", "-f", "ps-wg-"+inst.ID)
		}
	}

	s.ports.ReleaseInstance(inst.ID)
	inst.Status = "stopped"
	inst.containerID = ""
	if err := s.saveInstance(inst); err != nil {
		return WargameInstance{}, err
	}
	return inst, nil
}

func (s *Service) Extend(id string, addHours int) (WargameInstance, error) {
	if !isValidInstanceID(id) {
		return WargameInstance{}, ErrNotFound
	}
	if addHours <= 0 {
		addHours = s.cfg.DefaultTTLHours
	}
	if addHours > 72 {
		addHours = 72
	}

	mu := s.lockFor(id)
	mu.Lock()
	defer mu.Unlock()

	inst, err := s.GetInstance(id)
	if err != nil {
		return WargameInstance{}, err
	}
	base := time.Now().Unix()
	if inst.ExpiresAt > base {
		base = inst.ExpiresAt
	}
	inst.ExpiresAt = base + int64(addHours)*3600
	if err := s.saveInstance(inst); err != nil {
		return WargameInstance{}, err
	}
	return inst, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if !isValidInstanceID(id) {
		return ErrNotFound
	}
	if _, err := s.GetInstance(id); err != nil {
		return err
	}
	return s.Terminate(ctx, id)
}

func (s *Service) Terminate(ctx context.Context, id string) error {
	if !isValidInstanceID(id) {
		return nil
	}
	s.cancelBuild(id)

	mu := s.lockFor(id)
	mu.Lock()

	inst, getErr := s.GetInstance(id)
	s.ports.ReleaseInstance(id)

	instWorkDir := filepath.Join(s.cfg.WorkDir, id)
	if getErr == nil && inst.workDir != "" {
		instWorkDir = inst.workDir
	}

	if _, lookErr := exec.LookPath("docker"); lookErr == nil {
		srcDir := filepath.Join(instWorkDir, "src")
		composePS := filepath.Join(srcDir, "compose.ps.yaml")
		if _, statErr := os.Stat(composePS); statErr == nil {
			_ = runCmdQuiet(ctx, srcDir,
				"docker", "compose", "-p", "ps-wg-"+id, "-f", "compose.ps.yaml",
				"down", "--rmi", "local", "-v", "--remove-orphans",
			)
		}
		_ = runCmdQuiet(ctx, "", "docker", "rm", "-f", "ps-wg-"+id)
		_ = runCmdQuiet(ctx, "", "docker", "rmi", "-f", "ps-wg-"+id+":latest")
	}

	if isWithinBoundary(instWorkDir, s.cfg.WorkDir) && instWorkDir != s.cfg.WorkDir {
		_ = os.RemoveAll(instWorkDir)
	}
	_, dbErr := s.db.Exec(`DELETE FROM wargame_instances WHERE id = ?`, id)

	mu.Unlock()

	s.locksMu.Lock()
	delete(s.instanceLocks, id)
	s.locksMu.Unlock()

	return dbErr
}

func (s *Service) Logs(ctx context.Context, id string) (buildLog, containerLog, status string, err error) {
	inst, err := s.GetInstance(id)
	if err != nil {
		return "", "", "", err
	}
	buildLog = readTailString(filepath.Join(inst.workDir, "build.log"), maxLogReadBytes)

	if _, lookErr := exec.LookPath("docker"); lookErr == nil && (inst.Status == "running" || inst.Status == "stopped" || inst.Status == "failed") {
		logCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		if inst.Mode == "compose" {
			srcDir := filepath.Join(inst.workDir, "src")
			composePS := filepath.Join(srcDir, "compose.ps.yaml")
			if _, statErr := os.Stat(composePS); statErr == nil {
				out, cmdErr := runCmdCombined(logCtx, srcDir,
					"docker", "compose", "-p", "ps-wg-"+inst.ID, "-f", "compose.ps.yaml", "logs", "--no-color", "--tail=300",
				)
				if cmdErr == nil {
					containerLog = truncateTailBytes(out, maxLogReadBytes)
				}
			}
		} else {
			out, cmdErr := runCmdCombined(logCtx, "", "docker", "logs", "--tail=300", "ps-wg-"+inst.ID)
			if cmdErr == nil {
				containerLog = truncateTailBytes(out, maxLogReadBytes)
			}
		}
	}

	return buildLog, containerLog, inst.Status, nil
}

func (s *Service) SourceDir(id string) (srcReal, boundaryReal, downloadName string, err error) {
	inst, err := s.GetInstance(id)
	if err != nil {
		return "", "", "", err
	}
	srcCandidate := filepath.Join(inst.workDir, "src")
	srcReal, err = filepath.EvalSymlinks(srcCandidate)
	if err != nil {
		return "", "", "", ErrNotFound
	}
	boundaryReal, err = filepath.EvalSymlinks(s.cfg.WorkDir)
	if err != nil {
		return "", "", "", err
	}
	if !isWithinBoundary(srcReal, boundaryReal) {
		return "", "", "", ErrUnsafeArchiveEntry
	}
	return srcReal, boundaryReal, fmt.Sprintf("%s-%s.zip", sanitizeDownloadBase(inst.Name), inst.ID[:6]), nil
}

func sanitizeDownloadBase(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "wargame"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", "..", "_", "\"", "_")
	return replacer.Replace(trimmed)
}

func readTailString(path string, maxBytes int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	if st.Size() > maxBytes {
		_, _ = f.Seek(st.Size()-maxBytes, io.SeekStart)
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return ""
	}
	return string(raw)
}

func truncateTailBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return s[len(s)-maxBytes:]
}

func runCmdStreaming(ctx context.Context, dir string, out io.Writer, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var tailBuf bytes.Buffer
	mw := io.MultiWriter(out, &tailBuf)
	cmd.Stdout = mw
	cmd.Stderr = mw
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s 실패: %s", bin, lastNonEmptyLine(tailBuf.String(), err))
	}
	return nil
}

func runCmdCombined(ctx context.Context, dir string, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}

func runCmdQuiet(ctx context.Context, dir string, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.Run()
}

func firstLineOrErr(out string, fallback error) string {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return fallback.Error()
	}
	if idx := strings.IndexByte(trimmed, '\n'); idx != -1 {
		return strings.TrimSpace(trimmed[:idx])
	}
	return trimmed
}

func lastNonEmptyLine(out string, fallback error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return fallback.Error()
}

func mergeUniqueStrings(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, s := range base {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	for _, s := range extra {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

func nonNilPorts(in []PortBinding) []PortBinding {
	if in == nil {
		return []PortBinding{}
	}
	return in
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
