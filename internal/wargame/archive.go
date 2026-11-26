package wargame

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	ErrArchiveTooLarge    = errors.New("압축 해제 용량이 허용 한도를 초과했습니다")
	ErrUnsafeArchiveEntry = errors.New("보안상 허용되지 않는 압축 엔트리(경로 이탈 또는 심볼릭 링크)가 포함되어 있습니다")
	ErrUnsupportedArchive = errors.New("지원하지 않는 압축 형식입니다 (.zip, .tar.gz, .tgz, .tar.xz, .tar 지원)")
)

func IsSupportedArchive(filename string) bool {
	return archiveKind(filename) != ""
}

func StripArchiveExt(filename string) string {
	base := filepath.Base(strings.TrimSpace(filename))
	lower := strings.ToLower(base)
	for _, ext := range []string{".tar.gz", ".tar.xz", ".tgz", ".tar", ".zip"} {
		if strings.HasSuffix(lower, ext) && len(base) > len(ext) {
			return base[:len(base)-len(ext)]
		}
	}
	return base
}

func archiveKind(filename string) string {
	lower := strings.ToLower(strings.TrimSpace(filename))
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar.xz"):
		return "tar.xz"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	default:
		return ""
	}
}

func isWithinBoundary(targetReal, boundaryReal string) bool {
	if targetReal == boundaryReal {
		return true
	}
	prefix := boundaryReal
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(targetReal, prefix)
}

func cleanArchiveRelPath(rawName string) (string, error) {
	if strings.ContainsRune(rawName, '\x00') {
		return "", ErrUnsafeArchiveEntry
	}
	normalized := strings.ReplaceAll(rawName, "\\", "/")
	if strings.HasPrefix(normalized, "/") {
		return "", ErrUnsafeArchiveEntry
	}
	if len(normalized) >= 2 && normalized[1] == ':' {
		return "", ErrUnsafeArchiveEntry
	}
	for _, seg := range strings.Split(normalized, "/") {
		if seg == ".." {
			return "", ErrUnsafeArchiveEntry
		}
	}
	trimmed := strings.Trim(normalized, "/")
	if trimmed == "" || trimmed == "." {
		return "", nil
	}
	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	if cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", ErrUnsafeArchiveEntry
	}
	return cleaned, nil
}

func filePermFromArchive(mode fs.FileMode) os.FileMode {
	if mode&0o111 != 0 {
		return 0o755
	}
	return 0o644
}

func ExtractArchive(ctx context.Context, archivePath, dstDir string, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024 * 1024
	}
	kind := archiveKind(archivePath)
	if kind == "" {
		return ErrUnsupportedArchive
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create extract dir: %w", err)
	}
	dstAbs, err := filepath.Abs(dstDir)
	if err != nil {
		return err
	}
	dstReal, err := filepath.EvalSymlinks(dstAbs)
	if err != nil {
		return err
	}

	switch kind {
	case "zip":
		if err := extractZip(archivePath, dstReal, maxBytes); err != nil {
			return err
		}
	case "tar":
		f, err := os.Open(archivePath)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if err := extractTarStream(tar.NewReader(f), dstReal, maxBytes); err != nil {
			return err
		}
	case "tar.gz":
		f, err := os.Open(archivePath)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("open gzip stream: %w", err)
		}
		defer func() { _ = gz.Close() }()
		if err := extractTarStream(tar.NewReader(gz), dstReal, maxBytes); err != nil {
			return err
		}
	case "tar.xz":
		if err := extractTarXZ(ctx, archivePath, dstReal, maxBytes); err != nil {
			return err
		}
	}

	return UnwrapSingleDir(dstReal)
}

func extractZip(archivePath, dstReal string, maxBytes int64) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = zr.Close() }()

	var totalWritten int64
	for _, zf := range zr.File {
		rel, err := cleanArchiveRelPath(zf.Name)
		if err != nil {
			return err
		}
		mode := zf.Mode()
		if mode&os.ModeSymlink != 0 {
			return ErrUnsafeArchiveEntry
		}
		isDir := zf.FileInfo().IsDir() || strings.HasSuffix(zf.Name, "/")
		if !isDir && !mode.IsRegular() {
			return ErrUnsafeArchiveEntry
		}
		if rel == "" {
			continue
		}

		target := filepath.Join(dstReal, rel)
		if !isWithinBoundary(target, dstReal) {
			return ErrUnsafeArchiveEntry
		}
		if isDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		rc, err := zf.Open()
		if err != nil {
			return err
		}
		n, copyErr := writeBoundedFile(target, rc, filePermFromArchive(mode), maxBytes-totalWritten)
		_ = rc.Close()
		if copyErr != nil {
			return copyErr
		}
		totalWritten += n
	}
	return nil
}

func extractTarXZ(ctx context.Context, archivePath, dstReal string, maxBytes int64) error {
	xzBin, err := exec.LookPath("xz")
	if err != nil {
		return errors.New("호스트에 xz 명령어가 설치되어 있지 않아 .tar.xz 파일을 해제할 수 없습니다")
	}
	// Pipe xz stdout through Go's archive/tar reader so Zip Slip, symlink, and byte-limit checks run before any disk write.
	cmd := exec.CommandContext(ctx, xzBin, "-dc", archivePath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start xz: %w", err)
	}

	tarErr := extractTarStream(tar.NewReader(stdout), dstReal, maxBytes)
	if tarErr != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return tarErr
	}
	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("xz decompress: %s", msg)
		}
		return fmt.Errorf("xz decompress: %w", err)
	}
	return nil
}

func extractTarStream(tr *tar.Reader, dstReal string, maxBytes int64) error {
	var totalWritten int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}
		if hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}

		rel, err := cleanArchiveRelPath(hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if rel == "" {
				continue
			}
			target := filepath.Join(dstReal, rel)
			if !isWithinBoundary(target, dstReal) {
				return ErrUnsafeArchiveEntry
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if rel == "" {
				continue
			}
			if hdr.Size < 0 || hdr.Size > maxBytes-totalWritten {
				return ErrArchiveTooLarge
			}
			target := filepath.Join(dstReal, rel)
			if !isWithinBoundary(target, dstReal) {
				return ErrUnsafeArchiveEntry
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			n, copyErr := writeBoundedFile(target, tr, filePermFromArchive(hdr.FileInfo().Mode()), maxBytes-totalWritten)
			if copyErr != nil {
				return copyErr
			}
			totalWritten += n
		default:
			return ErrUnsafeArchiveEntry
		}
	}
}

func writeBoundedFile(target string, r io.Reader, perm os.FileMode, remaining int64) (int64, error) {
	if remaining < 0 {
		return 0, ErrArchiveTooLarge
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return 0, err
	}
	n, copyErr := io.Copy(f, io.LimitReader(r, remaining+1))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return n, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(target)
		return n, closeErr
	}
	if n > remaining {
		_ = os.Remove(target)
		return n, ErrArchiveTooLarge
	}
	return n, nil
}

func CopyDirSafe(srcDir, dstDir, rootBoundary string, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024 * 1024
	}
	srcReal, err := filepath.EvalSymlinks(srcDir)
	if err != nil {
		return err
	}
	boundaryReal, err := filepath.EvalSymlinks(rootBoundary)
	if err != nil {
		return err
	}
	if !isWithinBoundary(srcReal, boundaryReal) {
		return ErrUnsafeArchiveEntry
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	dstAbs, err := filepath.Abs(dstDir)
	if err != nil {
		return err
	}
	dstReal, err := filepath.EvalSymlinks(dstAbs)
	if err != nil {
		return err
	}

	var totalWritten int64
	err = filepath.WalkDir(srcReal, func(curr string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		resolved, evalErr := filepath.EvalSymlinks(curr)
		if evalErr != nil || !isWithinBoundary(resolved, boundaryReal) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, statErr := os.Stat(resolved)
		if statErr != nil {
			return nil
		}
		// Skip symlinked directories so in-tree links like `ln -s . loop` cannot recurse infinitely.
		if d.Type()&os.ModeSymlink != 0 && info.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(srcReal, curr)
		if relErr != nil || rel == "." {
			return nil
		}
		target := filepath.Join(dstReal, rel)
		if !isWithinBoundary(target, dstReal) {
			return ErrUnsafeArchiveEntry
		}

		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > maxBytes-totalWritten {
			return ErrArchiveTooLarge
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		in, openErr := os.Open(resolved)
		if openErr != nil {
			return openErr
		}
		n, copyErr := writeBoundedFile(target, in, filePermFromArchive(info.Mode()), maxBytes-totalWritten)
		_ = in.Close()
		if copyErr != nil {
			return copyErr
		}
		totalWritten += n
		return nil
	})
	if err != nil {
		return err
	}

	return UnwrapSingleDir(dstReal)
}

var topLevelMarkerFiles = map[string]struct{}{
	"Dockerfile":          {},
	"compose.yaml":        {},
	"compose.yml":         {},
	"docker-compose.yml":  {},
	"docker-compose.yaml": {},
}

func UnwrapSingleDir(srcDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	var dirs []string
	fileCount := 0
	for _, de := range entries {
		name := de.Name()
		if _, isMarker := topLevelMarkerFiles[name]; isMarker {
			return nil
		}
		if name == "__MACOSX" || name == ".DS_Store" {
			continue
		}
		if de.IsDir() {
			dirs = append(dirs, name)
		} else {
			fileCount++
		}
	}

	if fileCount != 0 || len(dirs) != 1 {
		return nil
	}

	_ = os.RemoveAll(filepath.Join(srcDir, "__MACOSX"))
	_ = os.RemoveAll(filepath.Join(srcDir, ".DS_Store"))

	onlySub := filepath.Join(srcDir, dirs[0])
	// Stage via a temporary directory because CTF archives commonly nest `chall/chall` (child sharing the wrapper's name).
	staging, err := os.MkdirTemp(srcDir, ".ps-unwrap-*")
	if err != nil {
		return err
	}
	_ = os.Remove(staging)
	if err := os.Rename(onlySub, staging); err != nil {
		return err
	}

	children, err := os.ReadDir(staging)
	if err != nil {
		return err
	}
	for _, child := range children {
		srcChild := filepath.Join(staging, child.Name())
		dstChild := filepath.Join(srcDir, child.Name())
		if err := os.Rename(srcChild, dstChild); err != nil {
			return err
		}
	}
	return os.Remove(staging)
}
