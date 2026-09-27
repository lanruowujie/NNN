// Package release contains deterministic helpers shared by release scripts.
package release

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

func LockedComponents(repositoryRoot string) ([]runtimebundle.ManifestComponent, error) {
	libnfc, err := readLock(filepath.Join(repositoryRoot, "third_party", "libnfc", "libnfc.lock"))
	if err != nil {
		return nil, err
	}
	mfoc, err := readLock(filepath.Join(repositoryRoot, "third_party", "mfoc", "mfoc.lock"))
	if err != nil {
		return nil, err
	}
	mfcuk, err := readLock(filepath.Join(repositoryRoot, "third_party", "mfcuk", "mfcuk.lock"))
	if err != nil {
		return nil, err
	}
	hardnested, err := readLock(filepath.Join(repositoryRoot, "third_party", "mfoc-hardnested", "hardnested.lock"))
	if err != nil {
		return nil, err
	}
	components := []runtimebundle.ManifestComponent{
		component("libnfc", "LIBNFC", libnfc),
		component("mfoc", "MFOC", mfoc),
		component("mfcuk", "MFCUK", mfcuk),
		component("mfoc-hardnested", "HARDNESTED", hardnested),
		{ID: "nfc-mfsetuid", Version: libnfc["LIBNFC_VERSION"], Commit: libnfc["LIBNFC_COMMIT"], License: libnfc["LIBNFC_UTILS_LICENSE"], Source: libnfc["LIBNFC_SOURCE_URL"]},
	}
	for _, item := range components {
		if item.Version == "" || item.Commit == "" || item.License == "" || item.Source == "" {
			return nil, fmt.Errorf("incomplete locked metadata for %s", item.ID)
		}
	}
	return components, nil
}

func component(id, prefix string, values map[string]string) runtimebundle.ManifestComponent {
	return runtimebundle.ManifestComponent{ID: id, Version: values[prefix+"_VERSION"], Commit: values[prefix+"_COMMIT"], License: values[prefix+"_LICENSE"], Source: values[prefix+"_SOURCE_URL"]}
}

func readLock(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("invalid lock line in %s: %q", path, line)
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
			return nil, fmt.Errorf("lock value %s in %s is not double quoted", name, path)
		}
		values[name] = strings.Trim(value, "\"")
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
