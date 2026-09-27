package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

var (
	ErrExecutableNotFound = errors.New("bundled executable not found")
	ErrUnsafeExecutable   = errors.New("unsafe bundled executable")
	ErrHashMismatch       = errors.New("bundled executable hash mismatch")
)

// Executable identifies one application-controlled runtime file. FileName and
// WindowsFileName must be base names, never paths supplied by a user.
type Executable struct {
	ID              string
	FileName        string
	WindowsFileName string
	SHA256          string
}

func (e Executable) platformFileName() string {
	if goruntime.GOOS == "windows" && e.WindowsFileName != "" {
		return e.WindowsFileName
	}
	return e.FileName
}

// Locator searches only the configured platform runtime roots. Each resolved
// symlink must still remain inside its root.
type Locator struct {
	roots []string
}

func NewLocator(roots ...string) *Locator {
	cleaned := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		absolute, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		absolute = filepath.Clean(absolute)
		if _, exists := seen[absolute]; exists {
			continue
		}
		seen[absolute] = struct{}{}
		cleaned = append(cleaned, absolute)
	}
	return &Locator{roots: cleaned}
}

// DefaultLocator supports both an installed application runtime and the
// repository-local runtime used by `wails dev`. It deliberately has no
// environment-variable override for executable directories.
func DefaultLocator() *Locator {
	platform := goruntime.GOOS + "-" + goruntime.GOARCH
	var roots []string
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		if goruntime.GOOS == "windows" {
			// Windows resolves imported DLLs from the executable directory before
			// Go code starts, so the portable ZIP keeps its private runtime flat.
			roots = append(roots, executableDir)
		}
		roots = append(roots,
			filepath.Join(executableDir, "runtime", platform),
			filepath.Join(executableDir, "..", "Resources", "runtime", platform),
		)
	}
	if workingDir, err := os.Getwd(); err == nil {
		roots = append(roots, filepath.Join(workingDir, "runtime", platform))
	}
	return NewLocator(roots...)
}

func (l *Locator) Roots() []string {
	if l == nil {
		return nil
	}
	return append([]string(nil), l.roots...)
}

// RuntimeRoot returns the first existing directory in the controlled runtime
// search order. It is intended for a child process environment, not as an
// overrideable executable search path.
func (l *Locator) RuntimeRoot() (string, error) {
	if l == nil {
		return "", fmt.Errorf("%w: locator is nil", ErrExecutableNotFound)
	}
	for _, root := range l.roots {
		info, err := os.Stat(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("inspect runtime root: %w", err)
		}
		if info.IsDir() {
			return root, nil
		}
	}
	return "", fmt.Errorf("%w: runtime directory", ErrExecutableNotFound)
}

// Resolve returns an absolute, verified path inside one configured runtime
// root. A missing file is distinct from a file that exists but is unsafe.
func (l *Locator) Resolve(executable Executable) (string, error) {
	if l == nil {
		return "", fmt.Errorf("%w: locator is nil", ErrExecutableNotFound)
	}
	name := executable.platformFileName()
	if executable.ID == "" || !validBaseName(name) {
		return "", fmt.Errorf("%w: invalid descriptor for %q", ErrUnsafeExecutable, executable.ID)
	}
	for _, root := range l.roots {
		path := filepath.Join(root, name)
		if _, err := os.Lstat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("inspect bundled executable %s: %w", executable.ID, err)
		}
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", fmt.Errorf("resolve runtime root: %w", err)
		}
		resolvedPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", fmt.Errorf("resolve bundled executable %s: %w", executable.ID, err)
		}
		inside, err := pathWithin(resolvedRoot, resolvedPath)
		if err != nil || !inside {
			return "", fmt.Errorf("%w: %s escapes its runtime root", ErrUnsafeExecutable, executable.ID)
		}
		info, err := os.Stat(resolvedPath)
		if err != nil {
			return "", fmt.Errorf("stat bundled executable %s: %w", executable.ID, err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("%w: %s is not a regular file", ErrUnsafeExecutable, executable.ID)
		}
		if goruntime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			return "", fmt.Errorf("%w: %s is not executable", ErrUnsafeExecutable, executable.ID)
		}
		if executable.SHA256 != "" {
			if err := verifyHash(resolvedPath, executable.SHA256); err != nil {
				return "", fmt.Errorf("%s: %w", executable.ID, err)
			}
		}
		return resolvedPath, nil
	}
	return "", fmt.Errorf("%w: %s", ErrExecutableNotFound, executable.ID)
}

func validBaseName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name &&
		!strings.ContainsAny(name, `/\\`) && !strings.ContainsRune(name, 0)
}

func pathWithin(root, path string) (bool, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false, err
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative), nil
}

func verifyHash(path, wanted string) error {
	wanted = strings.ToLower(strings.TrimSpace(wanted))
	if len(wanted) != sha256.Size*2 {
		return fmt.Errorf("%w: invalid expected SHA-256", ErrHashMismatch)
	}
	if _, err := hex.DecodeString(wanted); err != nil {
		return fmt.Errorf("%w: invalid expected SHA-256", ErrHashMismatch)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return err
	}
	actual := fmt.Sprintf("%x", digest.Sum(nil))
	if actual != wanted {
		return fmt.Errorf("%w: got %s", ErrHashMismatch, actual)
	}
	return nil
}
