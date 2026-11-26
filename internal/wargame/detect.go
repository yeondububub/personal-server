package wargame

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type PortBinding struct {
	Service       string `json:"service"`
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
	Protocol      string `json:"protocol"`
	ConnKind      string `json:"connKind"`
}

type DetectResult struct {
	Mode           string
	TemplateKind   string
	EntrypointFile string
	ComposeFile    string
	Ports          []PortBinding
	Warnings       []string
}

var composeCandidateNames = []string{
	"docker-compose.yml",
	"docker-compose.yaml",
	"compose.yml",
	"compose.yaml",
}

var httpKnownPorts = map[int]struct{}{
	80:   {},
	443:  {},
	3000: {},
	4000: {},
	5000: {},
	8000: {},
	8080: {},
	8888: {},
}

func Detect(srcDir string) (DetectResult, error) {
	for _, candidate := range composeCandidateNames {
		full := filepath.Join(srcDir, candidate)
		if st, err := os.Stat(full); err == nil && st.Mode().IsRegular() {
			return detectCompose(full, candidate)
		}
	}

	dockerfilePath := filepath.Join(srcDir, "Dockerfile")
	if st, err := os.Stat(dockerfilePath); err == nil && st.Mode().IsRegular() {
		return detectDockerfile(dockerfilePath)
	}

	return detectTemplate(srcDir)
}

func detectCompose(fullPath, relName string) (DetectResult, error) {
	raw, err := os.ReadFile(fullPath)
	if err != nil {
		return DetectResult{}, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return DetectResult{}, fmt.Errorf("compose YAML 파싱 오류: %w", err)
	}

	res := DetectResult{
		Mode:        "compose",
		ComposeFile: relName,
		Ports:       make([]PortBinding, 0),
		Warnings:    make([]string, 0),
	}

	servicesRaw, _ := doc["services"].(map[string]any)
	svcNames := make([]string, 0, len(servicesRaw))
	for name := range servicesRaw {
		svcNames = append(svcNames, name)
	}
	sort.Strings(svcNames)

	for _, svcName := range svcNames {
		svcMap, ok := servicesRaw[svcName].(map[string]any)
		if !ok {
			continue
		}
		for _, cPort := range parseComposePortList(svcMap["ports"]) {
			res.Ports = append(res.Ports, PortBinding{
				Service:       svcName,
				ContainerPort: cPort,
				HostPort:      0,
				Protocol:      "tcp",
				ConnKind:      inferComposeConnKind(svcName, cPort),
			})
		}
	}

	if len(res.Ports) == 0 {
		for _, svcName := range svcNames {
			svcMap, ok := servicesRaw[svcName].(map[string]any)
			if !ok {
				continue
			}
			for _, cPort := range parseComposePortList(svcMap["expose"]) {
				res.Ports = append(res.Ports, PortBinding{
					Service:       svcName,
					ContainerPort: cPort,
					HostPort:      0,
					Protocol:      "tcp",
					ConnKind:      inferComposeConnKind(svcName, cPort),
				})
			}
		}
		if len(res.Ports) == 0 {
			res.Warnings = append(res.Warnings, "Compose 파일에 외부 노출 포트(ports)가 정의되어 있지 않습니다.")
		}
	}

	return res, nil
}

func parseComposePortList(raw any) []int {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	seen := make(map[int]struct{})
	out := make([]int, 0, len(list))
	addPort := func(p int) {
		if p <= 0 || p > 65535 {
			return
		}
		if _, dup := seen[p]; dup {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}

	for _, item := range list {
		switch v := item.(type) {
		case int:
			addPort(v)
		case string:
			for _, p := range parseComposePortSpec(v) {
				addPort(p)
			}
		case map[string]any:
			if proto, ok := v["protocol"].(string); ok && strings.EqualFold(proto, "udp") {
				continue
			}
			switch t := v["target"].(type) {
			case int:
				addPort(t)
			case string:
				if p, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
					addPort(p)
				}
			}
		}
	}
	return out
}

func parseComposePortSpec(spec string) []int {
	s := strings.TrimSpace(spec)
	if s == "" {
		return nil
	}
	if slashIdx := strings.IndexByte(s, '/'); slashIdx != -1 {
		proto := strings.ToLower(strings.TrimSpace(s[slashIdx+1:]))
		if proto == "udp" {
			return nil
		}
		s = s[:slashIdx]
	}
	parts := strings.Split(s, ":")
	target := strings.TrimSpace(parts[len(parts)-1])
	if dashIdx := strings.IndexByte(target, '-'); dashIdx != -1 {
		startP, err1 := strconv.Atoi(strings.TrimSpace(target[:dashIdx]))
		endP, err2 := strconv.Atoi(strings.TrimSpace(target[dashIdx+1:]))
		if err1 == nil && err2 == nil && endP >= startP && endP-startP <= 16 {
			ports := make([]int, 0, endP-startP+1)
			for p := startP; p <= endP; p++ {
				ports = append(ports, p)
			}
			return ports
		}
		target = target[:dashIdx]
	}
	p, err := strconv.Atoi(target)
	if err != nil {
		return nil
	}
	return []int{p}
}

func inferComposeConnKind(serviceName string, containerPort int) string {
	if _, ok := httpKnownPorts[containerPort]; ok {
		return "http"
	}
	lower := strings.ToLower(serviceName)
	for _, hint := range []string{"web", "app", "nginx", "php"} {
		if strings.Contains(lower, hint) {
			return "http"
		}
	}
	return "nc"
}

func detectDockerfile(dockerfilePath string) (DetectResult, error) {
	raw, err := os.ReadFile(dockerfilePath)
	if err != nil {
		return DetectResult{}, err
	}
	content := string(raw)
	lower := strings.ToLower(content)

	connKind := "http"
	if strings.Contains(lower, "socat") || strings.Contains(lower, "xinetd") {
		connKind = "nc"
	}

	ports := parseDockerfileExpose(content)
	warnings := make([]string, 0)
	if len(ports) == 0 {
		ports = []int{8000}
		warnings = append(warnings, "Dockerfile에 EXPOSE 지시어가 없어 기본 포트 8000으로 설정했습니다.")
	}

	bindings := make([]PortBinding, 0, len(ports))
	for _, p := range ports {
		bindings = append(bindings, PortBinding{
			Service:       "default",
			ContainerPort: p,
			HostPort:      0,
			Protocol:      "tcp",
			ConnKind:      connKind,
		})
	}

	return DetectResult{
		Mode:     "dockerfile",
		Ports:    bindings,
		Warnings: warnings,
	}, nil
}

func parseDockerfileExpose(content string) []int {
	seen := make(map[int]struct{})
	out := make([]int, 0)
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if idx := strings.IndexByte(line, '#'); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "EXPOSE") {
			continue
		}
		for _, tok := range fields[1:] {
			proto := "tcp"
			portStr := tok
			if slashIdx := strings.IndexByte(tok, '/'); slashIdx != -1 {
				portStr = tok[:slashIdx]
				proto = strings.ToLower(tok[slashIdx+1:])
			}
			if proto != "tcp" {
				continue
			}
			p, err := strconv.Atoi(portStr)
			if err != nil || p <= 0 || p > 65535 {
				continue
			}
			if _, dup := seen[p]; !dup {
				seen[p] = struct{}{}
				out = append(out, p)
			}
		}
	}
	return out
}

func isIgnoredSharedLib(baseName string) bool {
	lower := strings.ToLower(baseName)
	if matched, _ := filepath.Match("libc*.so*", lower); matched {
		return true
	}
	if matched, _ := filepath.Match("ld-*.so*", lower); matched {
		return true
	}
	return false
}

func hasELFMagic(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	var hdr [4]byte
	n, err := io.ReadFull(f, hdr[:])
	if err != nil || n != 4 {
		return false
	}
	return bytes.Equal(hdr[:], []byte{0x7f, 'E', 'L', 'F'})
}

func detectTemplate(srcDir string) (DetectResult, error) {
	type fileCandidate struct {
		rel   string
		base  string
		depth int
		path  string
	}
	var candidates []fileCandidate
	_ = filepath.WalkDir(srcDir, func(curr string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "node_modules" || name == "__MACOSX" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || name == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(srcDir, curr)
		if err != nil {
			return nil
		}
		slashRel := filepath.ToSlash(rel)
		candidates = append(candidates, fileCandidate{
			rel:   slashRel,
			base:  name,
			depth: strings.Count(slashRel, "/"),
			path:  curr,
		})
		return nil
	})

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].depth != candidates[j].depth {
			return candidates[i].depth < candidates[j].depth
		}
		return candidates[i].rel < candidates[j].rel
	})

	var primaryELF, fallbackSharedELF string
	for _, c := range candidates {
		if isIgnoredSharedLib(c.base) {
			continue
		}
		if hasELFMagic(c.path) {
			lower := strings.ToLower(c.base)
			if strings.Contains(lower, ".so") || strings.HasSuffix(lower, ".o") {
				if fallbackSharedELF == "" {
					fallbackSharedELF = c.rel
				}
			} else if primaryELF == "" {
				primaryELF = c.rel
			}
		}
	}
	elfEntry := primaryELF
	if elfEntry == "" {
		elfEntry = fallbackSharedELF
	}
	if elfEntry != "" {
		return DetectResult{
			Mode:           "template",
			TemplateKind:   "pwn-socat",
			EntrypointFile: elfEntry,
			Ports: []PortBinding{{
				Service:       "default",
				ContainerPort: 9000,
				HostPort:      0,
				Protocol:      "tcp",
				ConnKind:      "nc",
			}},
			Warnings: make([]string, 0),
		}, nil
	}

	pyMarkers := map[string]int{"app.py": 1, "main.py": 2, "server.py": 3}
	bestPy := ""
	bestPyPrio := 99
	hasReqTxt := false
	anyPy := ""
	for _, c := range candidates {
		lower := strings.ToLower(c.base)
		if lower == "requirements.txt" {
			hasReqTxt = true
		}
		if prio, ok := pyMarkers[lower]; ok && prio < bestPyPrio {
			bestPy = c.rel
			bestPyPrio = prio
		} else if strings.HasSuffix(lower, ".py") && anyPy == "" {
			anyPy = c.rel
		}
	}
	if bestPy != "" || hasReqTxt {
		entry := bestPy
		if entry == "" {
			entry = anyPy
		}
		if entry == "" {
			entry = "app.py"
		}
		return DetectResult{
			Mode:           "template",
			TemplateKind:   "python-flask",
			EntrypointFile: entry,
			Ports: []PortBinding{{
				Service:       "default",
				ContainerPort: 5000,
				HostPort:      0,
				Protocol:      "tcp",
				ConnKind:      "http",
			}},
			Warnings: make([]string, 0),
		}, nil
	}

	phpEntry := ""
	for _, c := range candidates {
		lower := strings.ToLower(c.base)
		if lower == "index.php" {
			phpEntry = c.rel
			break
		}
		if strings.HasSuffix(lower, ".php") && phpEntry == "" {
			phpEntry = c.rel
		}
	}
	if phpEntry != "" {
		return DetectResult{
			Mode:           "template",
			TemplateKind:   "php-apache",
			EntrypointFile: phpEntry,
			Ports: []PortBinding{{
				Service:       "default",
				ContainerPort: 80,
				HostPort:      0,
				Protocol:      "tcp",
				ConnKind:      "http",
			}},
			Warnings: make([]string, 0),
		}, nil
	}

	for _, c := range candidates {
		if strings.EqualFold(c.base, "package.json") {
			return DetectResult{
				Mode:           "template",
				TemplateKind:   "node-express",
				EntrypointFile: c.rel,
				Ports: []PortBinding{{
					Service:       "default",
					ContainerPort: 3000,
					HostPort:      0,
					Protocol:      "tcp",
					ConnKind:      "http",
				}},
				Warnings: make([]string, 0),
			}, nil
		}
	}

	return DetectResult{
		Mode:           "template",
		TemplateKind:   "pwn-socat",
		EntrypointFile: "",
		Ports: []PortBinding{{
			Service:       "default",
			ContainerPort: 9000,
			HostPort:      0,
			Protocol:      "tcp",
			ConnKind:      "nc",
		}},
		Warnings: []string{"실행 구성을 자동 감지하지 못해 기본 pwn-socat 템플릿으로 설정했습니다."},
	}, nil
}
