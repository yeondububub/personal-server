package wargame

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"personal-server/internal/config"
	"personal-server/internal/db"
	"personal-server/internal/files"
)

func TestExtractZipRejectsTraversalSymlinkAndOversize(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	zipSlipPath := filepath.Join(tmp, "slip.zip")
	writeZipFile(t, zipSlipPath, []zipSpec{
		{name: "../../escape.txt", body: []byte("pwned"), mode: 0o644},
	})
	if err := ExtractArchive(ctx, zipSlipPath, filepath.Join(tmp, "out1"), 1024*1024); !errors.Is(err, ErrUnsafeArchiveEntry) {
		t.Fatalf("expected ErrUnsafeArchiveEntry for zip slip, got %v", err)
	}

	absZipPath := filepath.Join(tmp, "abs.zip")
	writeZipFile(t, absZipPath, []zipSpec{
		{name: "/etc/passwd", body: []byte("root"), mode: 0o644},
	})
	if err := ExtractArchive(ctx, absZipPath, filepath.Join(tmp, "out2"), 1024*1024); !errors.Is(err, ErrUnsafeArchiveEntry) {
		t.Fatalf("expected ErrUnsafeArchiveEntry for absolute zip entry, got %v", err)
	}

	symlinkZipPath := filepath.Join(tmp, "symlink.zip")
	writeZipFile(t, symlinkZipPath, []zipSpec{
		{name: "leak", body: []byte("/etc/shadow"), mode: os.ModeSymlink | 0o777},
	})
	if err := ExtractArchive(ctx, symlinkZipPath, filepath.Join(tmp, "out3"), 1024*1024); !errors.Is(err, ErrUnsafeArchiveEntry) {
		t.Fatalf("expected ErrUnsafeArchiveEntry for zip symlink, got %v", err)
	}

	bigZipPath := filepath.Join(tmp, "big.zip")
	writeZipFile(t, bigZipPath, []zipSpec{
		{name: "payload.bin", body: bytes.Repeat([]byte("A"), 2048), mode: 0o644},
	})
	if err := ExtractArchive(ctx, bigZipPath, filepath.Join(tmp, "out4"), 1024); !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("expected ErrArchiveTooLarge, got %v", err)
	}
}

func TestExtractTarRejectsSymlinkHardlinkAndSupportsTarXZ(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	tgzPath := filepath.Join(tmp, "sym.tar.gz")
	writeTarArchive(t, tgzPath, true, []*tar.Header{
		{Name: "chall/flag_link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777},
	}, nil)
	if err := ExtractArchive(ctx, tgzPath, filepath.Join(tmp, "out_sym"), 1024*1024); !errors.Is(err, ErrUnsafeArchiveEntry) {
		t.Fatalf("expected ErrUnsafeArchiveEntry for tar symlink, got %v", err)
	}

	hardlinkTar := filepath.Join(tmp, "hard.tar")
	writeTarArchive(t, hardlinkTar, false, []*tar.Header{
		{Name: "hard", Typeflag: tar.TypeLink, Linkname: "/etc/hosts", Mode: 0o644},
	}, nil)
	if err := ExtractArchive(ctx, hardlinkTar, filepath.Join(tmp, "out_hard"), 1024*1024); !errors.Is(err, ErrUnsafeArchiveEntry) {
		t.Fatalf("expected ErrUnsafeArchiveEntry for tar hardlink, got %v", err)
	}

	if xzBin, err := exec.LookPath("xz"); err == nil {
		rawTar := buildTarBytes(t, []*tar.Header{
			{Name: "prob/app.py", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len("from flask import Flask\n"))},
		}, [][]byte{[]byte("from flask import Flask\n")})
		txzPath := filepath.Join(tmp, "prob.tar.xz")
		compressWithXZ(t, xzBin, rawTar, txzPath)

		outXZ := filepath.Join(tmp, "out_xz")
		if err := ExtractArchive(ctx, txzPath, outXZ, 1024*1024); err != nil {
			t.Fatalf("expected valid .tar.xz to extract cleanly, got %v", err)
		}
		if _, err := os.Stat(filepath.Join(outXZ, "app.py")); err != nil {
			t.Fatalf("expected unwrapped app.py from .tar.xz, got %v", err)
		}

		evilRawTar := buildTarBytes(t, []*tar.Header{
			{Name: "evil_link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777},
		}, [][]byte{nil})
		evilTXZ := filepath.Join(tmp, "evil.tar.xz")
		compressWithXZ(t, xzBin, evilRawTar, evilTXZ)
		if err := ExtractArchive(ctx, evilTXZ, filepath.Join(tmp, "out_evil_xz"), 1024*1024); !errors.Is(err, ErrUnsafeArchiveEntry) {
			t.Fatalf("expected ErrUnsafeArchiveEntry for symlink inside .tar.xz, got %v", err)
		}
	}
}

func TestSingleFolderUnwrapSameChildNameAndCopyDirSafe(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()

	sameNameZip := filepath.Join(tmp, "chall.zip")
	writeZipFile(t, sameNameZip, []zipSpec{
		{name: "__MACOSX/._chall", body: []byte("mac-meta"), mode: 0o644},
		{name: ".DS_Store", body: []byte("ds"), mode: 0o644},
		{name: "chall/chall", body: []byte{0x7f, 'E', 'L', 'F', 1, 2}, mode: 0o755},
		{name: "chall/flag", body: []byte("flag{test}"), mode: 0o644},
	})
	dstUnwrapped := filepath.Join(tmp, "unwrapped")
	if err := ExtractArchive(ctx, sameNameZip, dstUnwrapped, 1024*1024); err != nil {
		t.Fatalf("unwrap extract failed: %v", err)
	}
	st, err := os.Stat(filepath.Join(dstUnwrapped, "chall"))
	if err != nil || st.IsDir() {
		t.Fatalf("expected regular binary file at unwrapped/chall, got st=%v err=%v", st, err)
	}

	rootBoundary := filepath.Join(tmp, "mount_root")
	srcChall := filepath.Join(rootBoundary, "web1")
	outsideSecret := filepath.Join(tmp, "outside_secret.txt")
	if err := os.MkdirAll(srcChall, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideSecret, []byte("TOP_SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcChall, "index.php"), []byte("<?php echo 'hi';"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideSecret, filepath.Join(srcChall, "escaped.txt")); err != nil {
		t.Fatal(err)
	}

	dstCopy := filepath.Join(tmp, "copied")
	if err := CopyDirSafe(srcChall, dstCopy, rootBoundary, 1024*1024); err != nil {
		t.Fatalf("CopyDirSafe failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstCopy, "index.php")); err != nil {
		t.Fatalf("expected index.php copied, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstCopy, "escaped.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected symlink escaping rootBoundary to be skipped, got err=%v", err)
	}
}

func TestDetectThreeStages(t *testing.T) {
	t.Run("compose priority and connKind inference", func(t *testing.T) {
		dir := t.TempDir()
		composeYAML := `
services:
  web:
    build: .
    ports:
      - "8080:80"
  pwn:
    build: ./pwn
    ports:
      - "31337:9999"
`
		if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(composeYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine\nEXPOSE 1234\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		res, err := Detect(dir)
		if err != nil {
			t.Fatal(err)
		}
		if res.Mode != "compose" || res.ComposeFile != "docker-compose.yml" || len(res.Ports) != 2 {
			t.Fatalf("unexpected compose detect result: %+v", res)
		}
		if res.Ports[0].Service != "pwn" || res.Ports[0].ContainerPort != 9999 || res.Ports[0].ConnKind != "nc" {
			t.Fatalf("expected pwn:9999 nc, got %+v", res.Ports[0])
		}
		if res.Ports[1].Service != "web" || res.Ports[1].ContainerPort != 80 || res.Ports[1].ConnKind != "http" {
			t.Fatalf("expected web:80 http, got %+v", res.Ports[1])
		}
	})

	t.Run("dockerfile socat vs default port fallback", func(t *testing.T) {
		dirSocat := t.TempDir()
		df := "FROM ubuntu:22.04\nRUN apt-get install -y socat\nEXPOSE 9001/tcp\n"
		if err := os.WriteFile(filepath.Join(dirSocat, "Dockerfile"), []byte(df), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := Detect(dirSocat)
		if err != nil {
			t.Fatal(err)
		}
		if res.Mode != "dockerfile" || len(res.Ports) != 1 || res.Ports[0].ContainerPort != 9001 || res.Ports[0].ConnKind != "nc" {
			t.Fatalf("unexpected socat dockerfile detection: %+v", res)
		}

		dirNoExpose := t.TempDir()
		if err := os.WriteFile(filepath.Join(dirNoExpose, "Dockerfile"), []byte("FROM python:3.11-slim\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		resFallback, err := Detect(dirNoExpose)
		if err != nil {
			t.Fatal(err)
		}
		if len(resFallback.Ports) != 1 || resFallback.Ports[0].ContainerPort != 8000 || resFallback.Ports[0].ConnKind != "http" || len(resFallback.Warnings) == 0 {
			t.Fatalf("expected default port 8000 http with warning, got %+v", resFallback)
		}
	})

	t.Run("template ELF ignores libc and ld", func(t *testing.T) {
		dir := t.TempDir()
		elf := []byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0}
		if err := os.WriteFile(filepath.Join(dir, "libc.so.6"), elf, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ld-linux-x86-64.so.2"), elf, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "vuln"), elf, 0o755); err != nil {
			t.Fatal(err)
		}
		res, err := Detect(dir)
		if err != nil {
			t.Fatal(err)
		}
		if res.Mode != "template" || res.TemplateKind != "pwn-socat" || res.EntrypointFile != "vuln" || res.Ports[0].ContainerPort != 9000 || res.Ports[0].ConnKind != "nc" {
			t.Fatalf("unexpected ELF template detection: %+v", res)
		}
	})

	t.Run("template python-flask php-apache node-express", func(t *testing.T) {
		pyDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(pyDir, "app.py"), []byte("from flask import Flask\n"), 0o644)
		pyRes, _ := Detect(pyDir)
		if pyRes.TemplateKind != "python-flask" || pyRes.Ports[0].ContainerPort != 5000 || pyRes.Ports[0].ConnKind != "http" {
			t.Fatalf("expected python-flask:5000, got %+v", pyRes)
		}

		phpDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(phpDir, "index.php"), []byte("<?php phpinfo();"), 0o644)
		phpRes, _ := Detect(phpDir)
		if phpRes.TemplateKind != "php-apache" || phpRes.Ports[0].ContainerPort != 80 {
			t.Fatalf("expected php-apache:80, got %+v", phpRes)
		}

		nodeDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(nodeDir, "package.json"), []byte(`{"name":"web"}`), 0o644)
		nodeRes, _ := Detect(nodeDir)
		if nodeRes.TemplateKind != "node-express" || nodeRes.Ports[0].ContainerPort != 3000 {
			t.Fatalf("expected node-express:3000, got %+v", nodeRes)
		}
	})

	t.Run("dockerfile ENV and ARG variable expansion in EXPOSE", func(t *testing.T) {
		cases := []struct {
			name       string
			dockerfile string
			wantPorts  []int
		}{
			{
				name:       "ENV key=value and $PORT",
				dockerfile: "FROM node:18\nENV PORT=3000\nEXPOSE $PORT\n",
				wantPorts:  []int{3000},
			},
			{
				name:       "ENV legacy syntax and ${PORT}/tcp",
				dockerfile: "FROM node:18\nENV PORT 3000\nEXPOSE ${PORT}/tcp\n",
				wantPorts:  []int{3000},
			},
			{
				name:       "ARG into ENV and multiline",
				dockerfile: "FROM alpine\nARG APP_PORT=\"4000\"\nENV HOST=0.0.0.0 \\\n    PORT=$APP_PORT\nEXPOSE ${PORT:-8000}\n",
				wantPorts:  []int{4000},
			},
			{
				name:       "default fallback syntax when unset",
				dockerfile: "FROM alpine\nEXPOSE ${UNSET_PORT:-3000}\n",
				wantPorts:  []int{3000},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(tc.dockerfile), 0o644); err != nil {
					t.Fatal(err)
				}
				res, err := Detect(dir)
				if err != nil {
					t.Fatal(err)
				}
				if res.Mode != "dockerfile" || len(res.Ports) != len(tc.wantPorts) {
					t.Fatalf("unexpected detect result: %+v", res)
				}
				for i, want := range tc.wantPorts {
					if res.Ports[i].ContainerPort != want {
						t.Fatalf("port[%d] = %d, want %d", i, res.Ports[i].ContainerPort, want)
					}
				}
			})
		}
	})
}

func TestSanitizeComposeStripsHostEscapesAndRewritesPorts(t *testing.T) {
	srcDir := t.TempDir()
	realSrc, err := filepath.EvalSymlinks(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(realSrc, "html"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc", filepath.Join(realSrc, "etc_sym")); err != nil {
		t.Fatal(err)
	}

	rawCompose := `
services:
  web:
    image: nginx:alpine
    privileged: true
    network_mode: host
    pid: host
    cap_add:
      - SYS_ADMIN
    devices:
      - "/dev/mem:/dev/mem"
    security_opt:
      - apparmor:unconfined
    volumes:
      - "/etc:/host_etc:ro"
      - "../outside:/outside"
      - "~/ssh:/root/.ssh"
      - "./etc_sym:/escaped_sym"
      - "./html:/usr/share/nginx/html:ro"
      - "db_data:/var/lib/mysql"
    ports:
      - "0.0.0.0:80:80"
  db:
    image: mysql:8
    ports:
      - "3306:3306"
volumes:
  db_data:
    driver_opts:
      type: none
      o: bind
      device: /etc
`

	allocs := []PortBinding{
		{Service: "web", ContainerPort: 80, HostPort: 30123, Protocol: "tcp", ConnKind: "http"},
	}

	sanitized, warnings, err := SanitizeCompose(realSrc, []byte(rawCompose), "100.64.0.9", allocs, false)
	if err != nil {
		t.Fatalf("SanitizeCompose failed: %v", err)
	}
	if len(warnings) < 8 {
		t.Fatalf("expected warnings for stripped fields and mounts, got %d: %v", len(warnings), warnings)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(sanitized, &parsed); err != nil {
		t.Fatal(err)
	}
	services := parsed["services"].(map[string]any)
	web := services["web"].(map[string]any)
	dbSvc := services["db"].(map[string]any)

	for _, forbidden := range []string{"privileged", "network_mode", "pid", "cap_add", "devices"} {
		if _, exists := web[forbidden]; exists {
			t.Fatalf("expected %s to be stripped from web service", forbidden)
		}
	}

	secOpts := web["security_opt"].([]any)
	if len(secOpts) != 1 || secOpts[0] != "no-new-privileges:true" {
		t.Fatalf("expected security_opt [no-new-privileges:true], got %v", secOpts)
	}
	if web["mem_limit"] != "256m" || web["pids_limit"] != 100 {
		t.Fatalf("expected resource limits injected, got mem=%v pids=%v", web["mem_limit"], web["pids_limit"])
	}

	vols := web["volumes"].([]any)
	if len(vols) != 2 || vols[0] != "./html:/usr/share/nginx/html:ro" || vols[1] != "db_data:/var/lib/mysql" {
		t.Fatalf("expected only safe ./html and db_data volumes kept, got %v", vols)
	}

	ports := web["ports"].([]any)
	if len(ports) != 1 || ports[0] != "100.64.0.9:30123:80/tcp" {
		t.Fatalf("expected rewritten port 100.64.0.9:30123:80/tcp, got %v", ports)
	}
	if _, hasDBPorts := dbSvc["ports"]; hasDBPorts {
		t.Fatalf("expected unallocated db service ports to be removed, got %v", dbSvc["ports"])
	}
}

func TestTemplatesStandaloneArgsAndStartupReconciliation(t *testing.T) {
	pwnDF, err := RenderTemplateDockerfile("pwn-socat", "chall", 9000)
	if err != nil || !strings.Contains(pwnDF, "USER ctf") || !strings.Contains(pwnDF, "flag{local_test_flag}") {
		t.Fatalf("unexpected pwn-socat Dockerfile: err=%v\n%s", err, pwnDF)
	}
	flaskDF, err := RenderTemplateDockerfile("python-flask", "app.py", 5000)
	if err != nil || !strings.Contains(flaskDF, "python app.py; exec python -m flask") {
		t.Fatalf("unexpected python-flask Dockerfile: err=%v\n%s", err, flaskDF)
	}

	strictPwn := WargameInstance{
		ID:           "a1b2c3d4e5f6",
		Mode:         "template",
		TemplateKind: "pwn-socat",
		Ports:        []PortBinding{{ContainerPort: 9000, HostPort: 30001}},
	}
	strictArgs := strings.Join(buildStandaloneRunArgs(strictPwn, "127.0.0.1"), " ")
	if !strings.Contains(strictArgs, "--cap-drop=ALL") || strings.Contains(strictArgs, "--cap-add=SETUID") {
		t.Fatalf("expected strict cap-drop without SETUID on pwn-socat:9000, got %s", strictArgs)
	}

	apacheInst := WargameInstance{
		ID:           "a1b2c3d4e5f6",
		Mode:         "template",
		TemplateKind: "php-apache",
		Ports:        []PortBinding{{ContainerPort: 80, HostPort: 30002}},
	}
	apacheArgs := strings.Join(buildStandaloneRunArgs(apacheInst, "127.0.0.1"), " ")
	if !strings.Contains(apacheArgs, "--cap-drop=ALL") || !strings.Contains(apacheArgs, "--cap-add=NET_BIND_SERVICE") {
		t.Fatalf("expected cap-drop=ALL plus NET_BIND_SERVICE for php-apache, got %s", apacheArgs)
	}

	relaxedInst := strictPwn
	relaxedInst.RelaxSecurity = true
	relaxedArgs := strings.Join(buildStandaloneRunArgs(relaxedInst, "127.0.0.1"), " ")
	if strings.Contains(relaxedArgs, "--cap-drop=ALL") {
		t.Fatalf("expected --cap-drop=ALL omitted when relaxSecurity=true, got %s", relaxedArgs)
	}

	tmp := t.TempDir()
	conn, err := db.Open(filepath.Join(tmp, "reconcile.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	now := time.Now().Unix()
	_, err = conn.Exec(
		`INSERT INTO wargame_instances (id, name, mode, status, work_dir, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"112233445566", "Interrupted Build", "dockerfile", "building", filepath.Join(tmp, "wg", "112233445566"), now+3600, now,
	)
	if err != nil {
		t.Fatal(err)
	}

	jail := files.NewJail([]config.MountRoot{{ID: "m", Name: "M", Path: tmp}})
	svc, err := NewService(conn, jail, config.WargameConfig{
		BindIP:          "127.0.0.1",
		PortStart:       32000,
		PortEnd:         32010,
		WorkDir:         filepath.Join(tmp, "wg"),
		DefaultTTLHours: 3,
		MaxArchiveBytes: 10 * 1024 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	reconciled, err := svc.GetInstance("112233445566")
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.Status != "failed" || reconciled.LastError != "서버 재시작으로 빌드가 중단되었습니다" {
		t.Fatalf("expected building instance reconciled to failed, got status=%q err=%q", reconciled.Status, reconciled.LastError)
	}
}

type zipSpec struct {
	name string
	body []byte
	mode os.FileMode
}

func writeZipFile(t *testing.T, dst string, entries []zipSpec) {
	t.Helper()
	f, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		hdr.SetMode(e.mode)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if len(e.body) > 0 {
			if _, err := w.Write(e.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func buildTarBytes(t *testing.T, headers []*tar.Header, bodies [][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for i, hdr := range headers {
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if i < len(bodies) && len(bodies[i]) > 0 {
			if _, err := tw.Write(bodies[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeTarArchive(t *testing.T, dst string, gzipCompress bool, headers []*tar.Header, bodies [][]byte) {
	t.Helper()
	raw := buildTarBytes(t, headers, bodies)
	f, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if gzipCompress {
		gz := gzip.NewWriter(f)
		if _, err := gz.Write(raw); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		if _, err := f.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func compressWithXZ(t *testing.T, xzBin string, raw []byte, dst string) {
	t.Helper()
	cmd := exec.Command(xzBin, "-c")
	cmd.Stdin = bytes.NewReader(raw)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("xz compress failed: %v", err)
	}
	if err := os.WriteFile(dst, out, 0o644); err != nil {
		t.Fatal(err)
	}
}
