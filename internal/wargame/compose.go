package wargame

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var dangerousComposeServiceKeys = []string{
	"privileged",
	"network_mode",
	"pid",
	"ipc",
	"uts",
	"userns_mode",
	"devices",
	"cap_add",
	"security_opt",
}

var namedVolumeRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func SanitizeCompose(srcDir string, rawYAML []byte, bindIP string, allocs []PortBinding, relaxSecurity bool) ([]byte, []string, error) {
	srcAbs, err := filepath.Abs(srcDir)
	if err != nil {
		return nil, nil, err
	}
	srcReal, err := filepath.EvalSymlinks(srcAbs)
	if err != nil {
		srcReal = srcAbs
	}

	var doc map[string]any
	if err := yaml.Unmarshal(rawYAML, &doc); err != nil {
		return nil, nil, fmt.Errorf("compose YAML 파싱 실패: %w", err)
	}
	if doc == nil {
		return nil, nil, errors.New("비어 있는 Compose 파일입니다")
	}

	servicesRaw, ok := doc["services"].(map[string]any)
	if !ok || len(servicesRaw) == 0 {
		return nil, nil, errors.New("Compose 파일에 services 항목이 없습니다")
	}

	warnings := make([]string, 0)

	if topVols, ok := doc["volumes"].(map[string]any); ok {
		volNames := make([]string, 0, len(topVols))
		for k := range topVols {
			volNames = append(volNames, k)
		}
		sort.Strings(volNames)
		for _, volName := range volNames {
			volCfg, ok := topVols[volName].(map[string]any)
			if !ok {
				continue
			}
			if _, hasOpts := volCfg["driver_opts"]; hasOpts {
				delete(volCfg, "driver_opts")
				warnings = append(warnings, fmt.Sprintf("볼륨 %s의 driver_opts 설정을 제거했습니다.", volName))
			}
			if ext, _ := volCfg["external"].(bool); ext {
				delete(volCfg, "external")
				warnings = append(warnings, fmt.Sprintf("볼륨 %s의 외부 볼륨 참조(external)를 제거했습니다.", volName))
			}
		}
	}

	portsByService := make(map[string][]string)
	for _, pb := range allocs {
		if pb.HostPort <= 0 || pb.ContainerPort <= 0 {
			continue
		}
		svc := pb.Service
		portsByService[svc] = append(portsByService[svc], fmt.Sprintf("%s:%d:%d/tcp", bindIP, pb.HostPort, pb.ContainerPort))
	}

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

		for _, key := range dangerousComposeServiceKeys {
			if _, exists := svcMap[key]; exists {
				delete(svcMap, key)
				warnings = append(warnings, fmt.Sprintf("서비스 %s에서 허용되지 않는 %s 설정을 제거했습니다.", svcName, key))
			}
		}

		if rawVols, hasVols := svcMap["volumes"]; hasVols {
			cleanedVols, volWarnings := sanitizeServiceVolumes(svcName, rawVols, srcReal)
			warnings = append(warnings, volWarnings...)
			if len(cleanedVols) > 0 {
				svcMap["volumes"] = cleanedVols
			} else {
				delete(svcMap, "volumes")
			}
		}

		if !relaxSecurity {
			svcMap["cpus"] = 0.5
			svcMap["mem_limit"] = "256m"
			svcMap["pids_limit"] = 100
			svcMap["security_opt"] = []string{"no-new-privileges:true"}
		}

		if rewritten := portsByService[svcName]; len(rewritten) > 0 {
			svcMap["ports"] = rewritten
		} else {
			delete(svcMap, "ports")
		}
	}

	outBytes, err := yaml.Marshal(doc)
	if err != nil {
		return nil, nil, fmt.Errorf("compose YAML 직렬화 실패: %w", err)
	}
	return outBytes, warnings, nil
}

func sanitizeServiceVolumes(svcName string, rawVols any, srcReal string) ([]any, []string) {
	list, ok := rawVols.([]any)
	if !ok {
		return nil, []string{fmt.Sprintf("서비스 %s의 비표준 volumes 형식을 제거했습니다.", svcName)}
	}

	kept := make([]any, 0, len(list))
	warnings := make([]string, 0)

	for _, item := range list {
		switch v := item.(type) {
		case string:
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				continue
			}
			if !strings.Contains(trimmed, ":") {
				if strings.HasPrefix(trimmed, "/") && !strings.Contains(trimmed, "..") {
					kept = append(kept, trimmed)
				} else {
					warnings = append(warnings, fmt.Sprintf("서비스 %s의 볼륨 마운트(%s)를 제거했습니다.", svcName, trimmed))
				}
				continue
			}
			parts := strings.SplitN(trimmed, ":", 2)
			source := strings.TrimSpace(parts[0])
			if isAllowedVolumeSource(source, srcReal) {
				kept = append(kept, trimmed)
			} else {
				warnings = append(warnings, fmt.Sprintf("서비스 %s의 호스트 바인드 마운트(%s)를 제거했습니다.", svcName, trimmed))
			}

		case map[string]any:
			volType, _ := v["type"].(string)
			source, _ := v["source"].(string)
			volType = strings.ToLower(strings.TrimSpace(volType))
			source = strings.TrimSpace(source)

			switch volType {
			case "tmpfs":
				kept = append(kept, v)
			case "volume", "":
				if source == "" || (namedVolumeRe.MatchString(source) && !strings.HasPrefix(source, ".")) {
					kept = append(kept, v)
				} else if isSafeRelativeWithinSrc(source, srcReal) {
					kept = append(kept, v)
				} else {
					warnings = append(warnings, fmt.Sprintf("서비스 %s의 호스트 바인드 마운트(%s)를 제거했습니다.", svcName, source))
				}
			case "bind":
				if isSafeRelativeWithinSrc(source, srcReal) {
					kept = append(kept, v)
				} else {
					warnings = append(warnings, fmt.Sprintf("서비스 %s의 호스트 바인드 마운트(%s)를 제거했습니다.", svcName, source))
				}
			default:
				warnings = append(warnings, fmt.Sprintf("서비스 %s의 %s 타입 볼륨을 제거했습니다.", svcName, volType))
			}
		}
	}
	return kept, warnings
}

func isAllowedVolumeSource(source, srcReal string) bool {
	if source == "" {
		return false
	}
	if strings.HasPrefix(source, "/") || strings.HasPrefix(source, "..") || strings.HasPrefix(source, "~") {
		return false
	}
	if namedVolumeRe.MatchString(source) && !strings.HasPrefix(source, ".") && !strings.Contains(source, "/") && !strings.Contains(source, "\\") {
		return true
	}
	return isSafeRelativeWithinSrc(source, srcReal)
}

func isSafeRelativeWithinSrc(source, srcReal string) bool {
	if source == "" || strings.ContainsRune(source, '\x00') {
		return false
	}
	normalized := strings.ReplaceAll(source, "\\", "/")
	if strings.HasPrefix(normalized, "/") || strings.HasPrefix(normalized, "..") || strings.HasPrefix(normalized, "~") {
		return false
	}
	if !strings.HasPrefix(normalized, "./") && normalized != "." {
		return false
	}
	joined := filepath.Join(srcReal, filepath.FromSlash(normalized))
	if !isWithinBoundary(joined, srcReal) {
		return false
	}

	probe := joined
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			return isWithinBoundary(resolved, srcReal)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return false
		}
		probe = parent
	}
}

func WriteSanitizedCompose(srcDir, composeFile, bindIP string, allocs []PortBinding, relaxSecurity bool) ([]string, error) {
	inPath := filepath.Join(srcDir, composeFile)
	raw, err := os.ReadFile(inPath)
	if err != nil {
		return nil, fmt.Errorf("read compose file: %w", err)
	}
	sanitized, warnings, err := SanitizeCompose(srcDir, raw, bindIP, allocs, relaxSecurity)
	if err != nil {
		return nil, err
	}
	outPath := filepath.Join(srcDir, "compose.ps.yaml")
	if err := os.WriteFile(outPath, sanitized, 0o644); err != nil {
		return nil, fmt.Errorf("write compose.ps.yaml: %w", err)
	}
	return warnings, nil
}
