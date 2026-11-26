package files

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"personal-server/internal/config"
)

var (
	ErrUnknownRoot   = errors.New("존재하지 않는 마운트 경로입니다")
	ErrPathTraversal = errors.New("허용된 디렉토리 범위를 벗어난 경로입니다")
)

type Jail struct {
	rootsOrder []config.MountRoot
	rootsMap   map[string]config.MountRoot
}

func NewJail(roots []config.MountRoot) *Jail {
	m := make(map[string]config.MountRoot, len(roots))
	copied := make([]config.MountRoot, len(roots))
	copy(copied, roots)
	for _, r := range copied {
		m[r.ID] = r
	}
	return &Jail{
		rootsOrder: copied,
		rootsMap:   m,
	}
}

func (j *Jail) Roots() []config.MountRoot {
	out := make([]config.MountRoot, len(j.rootsOrder))
	copy(out, j.rootsOrder)
	return out
}

func (j *Jail) Root(id string) (config.MountRoot, bool) {
	r, ok := j.rootsMap[id]
	return r, ok
}

func sanitizeRelPath(rel string) (string, error) {
	if strings.ContainsRune(rel, '\x00') {
		return "", ErrPathTraversal
	}
	normalized := strings.ReplaceAll(rel, "\\", "/")
	for _, seg := range strings.Split(normalized, "/") {
		if seg == ".." {
			return "", ErrPathTraversal
		}
	}
	cleaned := filepath.Clean("/" + normalized)
	trimmed := strings.TrimPrefix(cleaned, string(filepath.Separator))
	if trimmed == "." {
		return "", nil
	}
	return filepath.ToSlash(trimmed), nil
}

func isWithinRoot(targetReal, rootReal string) bool {
	if targetReal == rootReal {
		return true
	}
	prefix := rootReal
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(targetReal, prefix)
}

// ResolveExisting validates rootID + relPath and resolves symlinks, ensuring the target exists inside root.
func (j *Jail) ResolveExisting(rootID, relPath string) (realPath string, cleanRel string, err error) {
	root, ok := j.rootsMap[rootID]
	if !ok {
		return "", "", ErrUnknownRoot
	}
	cleanRel, err = sanitizeRelPath(relPath)
	if err != nil {
		return "", "", err
	}
	candidate := filepath.Join(root.Path, filepath.FromSlash(cleanRel))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", "", err
	}
	if !isWithinRoot(resolved, root.Path) {
		return "", "", ErrPathTraversal
	}
	return resolved, cleanRel, nil
}

// ResolveCreateTarget validates the parent directory via EvalSymlinks and returns a safe path for writing/creating/renaming.
func (j *Jail) ResolveCreateTarget(rootID, relPath string) (targetPath string, cleanRel string, err error) {
	root, ok := j.rootsMap[rootID]
	if !ok {
		return "", "", ErrUnknownRoot
	}
	cleanRel, err = sanitizeRelPath(relPath)
	if err != nil {
		return "", "", err
	}
	if cleanRel == "" {
		return "", "", fmt.Errorf("%w: 루트 자체는 생성/덮어쓰기 대상이 될 수 없습니다", ErrPathTraversal)
	}

	candidate := filepath.Join(root.Path, filepath.FromSlash(cleanRel))
	parentReal, err := filepath.EvalSymlinks(filepath.Dir(candidate))
	if err != nil {
		return "", "", err
	}
	if !isWithinRoot(parentReal, root.Path) {
		return "", "", ErrPathTraversal
	}

	base := filepath.Base(candidate)
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return "", "", ErrPathTraversal
	}

	dest := filepath.Join(parentReal, base)
	if info, lerr := os.Lstat(dest); lerr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			resolvedDest, evalErr := filepath.EvalSymlinks(dest)
			if evalErr != nil || !isWithinRoot(resolvedDest, root.Path) {
				return "", "", ErrPathTraversal
			}
			return resolvedDest, cleanRel, nil
		}
	}
	return dest, cleanRel, nil
}
