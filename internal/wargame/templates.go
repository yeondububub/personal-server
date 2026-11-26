package wargame

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func IsValidTemplateKind(kind string) bool {
	switch kind {
	case "pwn-socat", "python-flask", "php-apache", "node-express":
		return true
	default:
		return false
	}
}

func sanitizeEntrypointRel(raw, fallback string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = fallback
	}
	if trimmed == "" {
		return "", nil
	}
	if strings.ContainsAny(trimmed, "\"'`$\n\r;|&") {
		return "", errors.New("유효하지 않은 엔트리포인트 파일명입니다")
	}
	rel, err := cleanArchiveRelPath(trimmed)
	if err != nil || rel == "" {
		return "", errors.New("유효하지 않은 엔트리포인트 경로입니다")
	}
	return filepath.ToSlash(rel), nil
}

func RenderTemplateDockerfile(templateKind, entrypoint string, port int) (string, error) {
	if port <= 0 || port > 65535 {
		port = defaultPortForTemplate(templateKind)
	}

	switch templateKind {
	case "pwn-socat":
		ep, err := sanitizeEntrypointRel(entrypoint, "chall")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`FROM ubuntu:22.04
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends socat ca-certificates && rm -rf /var/lib/apt/lists/*
RUN useradd -m -s /bin/bash ctf
COPY . /home/ctf/
RUN if [ ! -f /home/ctf/flag ]; then echo 'flag{local_test_flag}' > /home/ctf/flag; fi \
    && if [ -f "/home/ctf/%s" ]; then chmod +x "/home/ctf/%s"; fi \
    && chmod -R a+rX /home/ctf
USER ctf
WORKDIR /home/ctf
EXPOSE %d
CMD ["socat", "TCP-LISTEN:%d,reuseaddr,fork", "EXEC:./%s,stderr"]
`, ep, ep, port, port, ep), nil

	case "python-flask":
		ep, err := sanitizeEntrypointRel(entrypoint, "app.py")
		if err != nil {
			return "", err
		}
		// Sequence with `;` rather than `||` because a Flask module without `app.run()` exits 0 on `python <ep>`.
		return fmt.Sprintf(`FROM python:3.11-slim
WORKDIR /app
COPY . /app
RUN if [ -f requirements.txt ]; then pip install --no-cache-dir -r requirements.txt; else pip install --no-cache-dir flask; fi
ENV FLASK_APP=%s
ENV PORT=%d
EXPOSE %d
CMD ["sh", "-c", "python %s; exec python -m flask --app %s run --host=0.0.0.0 --port=%d"]
`, ep, port, port, ep, ep, port), nil

	case "php-apache":
		apachePortCfg := ""
		if port != 80 {
			apachePortCfg = fmt.Sprintf("\nRUN sed -i 's/80/%d/g' /etc/apache2/ports.conf /etc/apache2/sites-available/000-default.conf", port)
		}
		return fmt.Sprintf(`FROM php:8.2-apache
COPY . /var/www/html/%s
EXPOSE %d
`, apachePortCfg, port), nil

	case "node-express":
		return fmt.Sprintf(`FROM node:20-slim
WORKDIR /app
COPY . /app
RUN if [ -f package.json ]; then npm install --omit=dev; fi
ENV PORT=%d
EXPOSE %d
CMD ["npm", "start"]
`, port, port), nil

	default:
		return "", fmt.Errorf("알 수 없는 템플릿 종류입니다: %s", templateKind)
	}
}

func defaultPortForTemplate(templateKind string) int {
	switch templateKind {
	case "python-flask":
		return 5000
	case "php-apache":
		return 80
	case "node-express":
		return 3000
	default:
		return 9000
	}
}

func WriteTemplateDockerfile(srcDir, templateKind, entrypoint string, port int) error {
	content, err := RenderTemplateDockerfile(templateKind, entrypoint, port)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(srcDir, "Dockerfile.ps"), []byte(content), 0o644)
}
