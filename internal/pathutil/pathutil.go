package pathutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrOutsideBase = errors.New("path resolves outside the configured base")

func ResolveLocal(base string, rel string) (string, error) {
	if rel == "" {
		return "", errors.New("relative path is empty")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: absolute paths are not allowed: %s", ErrOutsideBase, rel)
	}

	cleanBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve local base: %w", err)
	}
	cleanBase = filepath.Clean(cleanBase)

	full := filepath.Join(cleanBase, rel)
	full = filepath.Clean(full)

	if !isWithin(cleanBase, full) {
		return "", fmt.Errorf("%w: %s", ErrOutsideBase, rel)
	}
	return full, nil
}

func ResolveRemote(base string, rel string) (string, error) {
	if rel == "" {
		return "", errors.New("relative path is empty")
	}

	cleanRel := strings.ReplaceAll(rel, "\\", "/")
	if strings.HasPrefix(cleanRel, "/") {
		return "", fmt.Errorf("%w: absolute paths are not allowed: %s", ErrOutsideBase, rel)
	}

	cleanBase := cleanRemote(base)

	// Split into segments and normalize . and .. without escaping base
	segments := strings.Split(cleanRel, "/")
	normalized := make([]string, 0, len(segments))
	for _, seg := range segments {
		switch seg {
		case "", ".":
			continue
		case "..":
			if len(normalized) == 0 {
				return "", fmt.Errorf("%w: %s", ErrOutsideBase, rel)
			}
			normalized = normalized[:len(normalized)-1]
		default:
			normalized = append(normalized, seg)
		}
	}

	full := cleanRemote(cleanBase + "/" + strings.Join(normalized, "/"))

	if !isWithinRemote(cleanBase, full) {
		return "", fmt.Errorf("%w: %s", ErrOutsideBase, rel)
	}
	return full, nil
}

func ValidateLocalFile(base, rel string) (string, error) {
	full, err := ResolveLocal(base, rel)
	if err != nil {
		return "", err
	}

	cleanBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve local base: %w", err)
	}
	cleanBase = filepath.Clean(cleanBase)

	resolvedBase, err := filepath.EvalSymlinks(cleanBase)
	if err != nil {
		return "", fmt.Errorf("resolve local base symlinks: %w", err)
	}
	resolvedBase = filepath.Clean(resolvedBase)

	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("local file does not exist: %s", full)
		}
		return "", fmt.Errorf("resolve local file %s: %w", full, err)
	}

	if !isWithin(resolvedBase, resolved) {
		return "", fmt.Errorf("%w: path %s resolves outside the local base", ErrOutsideBase, full)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat local file %s: %w", resolved, err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("path is a directory: %s", full)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("path is not a regular file: %s", full)
	}

	f, err := os.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("open local file %s: %w", resolved, err)
	}
	f.Close()

	return resolved, nil
}

func cleanRemote(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimSuffix(p, "/")
}

func isWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func isWithinRemote(base, target string) bool {
	if base == "/" {
		return strings.HasPrefix(target, "/")
	}
	return target == base || strings.HasPrefix(target, base+"/")
}
