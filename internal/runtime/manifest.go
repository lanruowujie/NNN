package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const ManifestSchemaVersion = 1

var ErrInvalidManifest = errors.New("invalid runtime manifest")

type Manifest struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Platform      string              `json:"platform"`
	NFCXVersion   string              `json:"nfcxVersion"`
	Commit        string              `json:"commit"`
	Components    []ManifestComponent `json:"components"`
	Files         []ManifestFile      `json:"files"`
}

type ManifestComponent struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	License string `json:"license"`
	Source  string `json:"source"`
}

type ManifestFile struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	Executable bool   `json:"executable"`
}

type ManifestVerification struct {
	Manifest Manifest `json:"manifest"`
	Verified int      `json:"verified"`
}

func ReadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: decode JSON: %v", ErrInvalidManifest, err)
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// VerifyManifest validates every listed regular file and rejects paths that
// could escape the application-controlled runtime directory.
func VerifyManifest(root, manifestPath string) (ManifestVerification, error) {
	manifest, err := ReadManifest(manifestPath)
	if err != nil {
		return ManifestVerification{}, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return ManifestVerification{}, err
	}
	expected := make(map[string]struct{}, len(manifest.Files))
	for _, entry := range manifest.Files {
		expected[entry.Path] = struct{}{}
		path := filepath.Join(root, filepath.FromSlash(entry.Path))
		inside, relErr := pathWithin(root, path)
		if relErr != nil || !inside {
			return ManifestVerification{}, fmt.Errorf("%w: file %q escapes runtime root", ErrInvalidManifest, entry.Path)
		}
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return ManifestVerification{}, fmt.Errorf("verify %s: %w", entry.Path, statErr)
		}
		if !info.Mode().IsRegular() {
			return ManifestVerification{}, fmt.Errorf("%w: %s is not a regular file", ErrInvalidManifest, entry.Path)
		}
		if info.Size() != entry.Size {
			return ManifestVerification{}, fmt.Errorf("%w: %s size mismatch: got %d, want %d", ErrHashMismatch, entry.Path, info.Size(), entry.Size)
		}
		if err := verifyHash(path, entry.SHA256); err != nil {
			return ManifestVerification{}, fmt.Errorf("verify %s: %w", entry.Path, err)
		}
		if entry.Executable && info.Mode().Perm()&0o111 == 0 && filepath.Ext(path) != ".exe" {
			return ManifestVerification{}, fmt.Errorf("%w: %s lost its executable permission", ErrInvalidManifest, entry.Path)
		}
	}
	manifestAbsolute, err := filepath.Abs(manifestPath)
	if err != nil {
		return ManifestVerification{}, err
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if absolute == manifestAbsolute {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return fmt.Errorf("%w: resolve symlink %s: %v", ErrInvalidManifest, path, err)
			}
			inside, err := pathWithin(root, resolved)
			if err != nil || !inside {
				return fmt.Errorf("%w: symlink %s escapes runtime root", ErrInvalidManifest, path)
			}
			resolvedRelative, err := filepath.Rel(root, resolved)
			if err != nil {
				return err
			}
			if _, ok := expected[filepath.ToSlash(resolvedRelative)]; !ok {
				return fmt.Errorf("%w: symlink %s targets an unlisted file", ErrInvalidManifest, path)
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if _, ok := expected[filepath.ToSlash(relative)]; !ok {
			return fmt.Errorf("%w: unlisted runtime file %s", ErrInvalidManifest, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return ManifestVerification{}, err
	}
	return ManifestVerification{Manifest: manifest, Verified: len(manifest.Files)}, nil
}

// BuildManifest describes all regular files below root except the output
// manifest itself. Symlinks are intentionally excluded: release packages must
// contain and hash the real loadable files.
func BuildManifest(root, outputName, platform, version, commit string, components []ManifestComponent) (Manifest, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{SchemaVersion: ManifestSchemaVersion, Platform: platform, NFCXVersion: version, Commit: commit, Components: append([]ManifestComponent(nil), components...)}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == filepath.ToSlash(outputName) {
			return nil
		}
		digest, err := hashFile(path)
		if err != nil {
			return err
		}
		manifest.Files = append(manifest.Files, ManifestFile{Path: relative, SHA256: digest, Size: info.Size(), Executable: info.Mode().Perm()&0o111 != 0 || strings.EqualFold(filepath.Ext(relative), ".exe")})
		return nil
	})
	if err != nil {
		return Manifest{}, err
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func WriteManifest(path string, manifest Manifest) error {
	if err := validateManifest(manifest); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func validateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("%w: unsupported schema version %d", ErrInvalidManifest, manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.Platform) == "" || strings.TrimSpace(manifest.NFCXVersion) == "" || strings.TrimSpace(manifest.Commit) == "" {
		return fmt.Errorf("%w: platform, NFCX version, and commit are required", ErrInvalidManifest)
	}
	seen := make(map[string]struct{}, len(manifest.Files))
	for _, entry := range manifest.Files {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(entry.Path)))
		if entry.Path == "" || clean != entry.Path || strings.HasPrefix(clean, "../") || filepath.IsAbs(filepath.FromSlash(entry.Path)) {
			return fmt.Errorf("%w: unsafe file path %q", ErrInvalidManifest, entry.Path)
		}
		if _, exists := seen[entry.Path]; exists {
			return fmt.Errorf("%w: duplicate file %q", ErrInvalidManifest, entry.Path)
		}
		seen[entry.Path] = struct{}{}
		if entry.Size < 0 || len(entry.SHA256) != sha256.Size*2 {
			return fmt.Errorf("%w: invalid metadata for %q", ErrInvalidManifest, entry.Path)
		}
		if _, err := hex.DecodeString(entry.SHA256); err != nil {
			return fmt.Errorf("%w: invalid SHA-256 for %q", ErrInvalidManifest, entry.Path)
		}
	}
	for _, component := range manifest.Components {
		if component.ID == "" || component.Version == "" || component.License == "" || component.Source == "" {
			return fmt.Errorf("%w: incomplete component metadata for %q", ErrInvalidManifest, component.ID)
		}
	}
	return nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}
