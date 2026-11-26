package files

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"personal-server/internal/config"
)

var (
	ErrUnknownRoot   = errors.New("존재하지 않는 마운트 경로입니다")
	ErrPathTraversal = errors.New("허용된 디렉토리 범위를 벗어난 경로입니다")
	ErrReadOnlyMount = errors.New("읽기 전용으로 마운트된 폴더에서는 수정할 수 없습니다")
	ErrPrivateMount  = errors.New("Tailscale 내부망 전용 마운트 폴더입니다")
)

type Jail struct {
	mu         sync.RWMutex
	rootsOrder []config.MountRoot
	rootsMap   map[string]config.MountRoot
}

func NewJail(roots []config.MountRoot) *Jail {
	j := &Jail{}
	j.Reload(roots)
	return j
}

func (j *Jail) Reload(roots []config.MountRoot) {
	m := make(map[string]config.MountRoot, len(roots))
	copied := make([]config.MountRoot, len(roots))
	copy(copied, roots)
	for _, r := range copied {
		m[r.ID] = r
	}
	j.mu.Lock()
	j.rootsOrder = copied
	j.rootsMap = m
	j.mu.Unlock()
}

func (j *Jail) Roots() []config.MountRoot {
	j.mu.RLock()
	defer j.mu.RUnlock()
	out := make([]config.MountRoot, len(j.rootsOrder))
	copy(out, j.rootsOrder)
	return out
}

func (j *Jail) RootsForTier(accessTier string) []config.MountRoot {
	j.mu.RLock()
	defer j.mu.RUnlock()
	out := make([]config.MountRoot, 0, len(j.rootsOrder))
	for _, r := range j.rootsOrder {
		if accessTier != "private" && r.PrivateOnly {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (j *Jail) Root(id string) (config.MountRoot, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	r, ok := j.rootsMap[id]
	return r, ok
}

func (j *Jail) rootForTier(id, accessTier string) (config.MountRoot, error) {
	j.mu.RLock()
	r, ok := j.rootsMap[id]
	j.mu.RUnlock()
	if !ok {
		return config.MountRoot{}, ErrUnknownRoot
	}
	if accessTier != "private" && r.PrivateOnly {
		return config.MountRoot{}, ErrPrivateMount
	}
	return r, nil
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

// ResolveExisting validates rootID + relPath (private/unrestricted tier) and resolves symlinks.
func (j *Jail) ResolveExisting(rootID, relPath string) (realPath string, cleanRel string, err error) {
	return j.ResolveExistingForTier(rootID, relPath, "private")
}

// ResolveExistingForTier validates rootID + relPath for the given accessTier ("public" or "private").
func (j *Jail) ResolveExistingForTier(rootID, relPath, accessTier string) (realPath string, cleanRel string, err error) {
	root, err := j.rootForTier(rootID, accessTier)
	if err != nil {
		return "", "", err
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

// ResolveCreateTarget validates the parent directory via EvalSymlinks (private tier) for writing/creating/renaming.
func (j *Jail) ResolveCreateTarget(rootID, relPath string) (targetPath string, cleanRel string, err error) {
	return j.ResolveCreateTargetForTier(rootID, relPath, "private")
}

// ResolveCreateTargetForTier validates tier visibility, read-only status, and parent symlinks for mutating operations.
func (j *Jail) ResolveCreateTargetForTier(rootID, relPath, accessTier string) (targetPath string, cleanRel string, err error) {
	root, err := j.rootForTier(rootID, accessTier)
	if err != nil {
		return "", "", err
	}
	if root.ReadOnly {
		return "", "", ErrReadOnlyMount
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

// CheckWritable verifies that rootID is accessible in accessTier and is not read-only.
func (j *Jail) CheckWritable(rootID, accessTier string) error {
	root, err := j.rootForTier(rootID, accessTier)
	if err != nil {
		return err
	}
	if root.ReadOnly {
		return ErrReadOnlyMount
	}
	return nil
}
