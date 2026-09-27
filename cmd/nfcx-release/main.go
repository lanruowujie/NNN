package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	nfcxrelease "github.com/BennyThink/NFCX/internal/release"
	runtimebundle "github.com/BennyThink/NFCX/internal/runtime"
)

var releaseVersionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, output io.Writer) error {
	if len(arguments) == 0 {
		return errors.New("usage: nfcx-release manifest|verify|checksums|scan-paths|set-version")
	}
	switch arguments[0] {
	case "manifest":
		flags := flag.NewFlagSet("manifest", flag.ContinueOnError)
		flags.SetOutput(output)
		root := flags.String("root", "", "runtime directory")
		repository := flags.String("repository", ".", "repository root")
		platform := flags.String("platform", "", "GOOS-GOARCH")
		version := flags.String("version", "", "NFCX version")
		commit := flags.String("commit", "", "Git commit")
		manifestPath := flags.String("output", "", "manifest path")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if *root == "" || *platform == "" || *version == "" || *commit == "" {
			return errors.New("manifest requires -root, -platform, -version, and -commit")
		}
		if *manifestPath == "" {
			*manifestPath = filepath.Join(*root, "manifest.json")
		}
		components, err := nfcxrelease.LockedComponents(*repository)
		if err != nil {
			return err
		}
		relativeOutput, err := filepath.Rel(*root, *manifestPath)
		if err != nil {
			return err
		}
		manifest, err := runtimebundle.BuildManifest(*root, relativeOutput, *platform, *version, *commit, components)
		if err != nil {
			return err
		}
		if err := runtimebundle.WriteManifest(*manifestPath, manifest); err != nil {
			return err
		}
		fmt.Fprintf(output, "wrote %s with %d files\n", *manifestPath, len(manifest.Files))
		return nil
	case "verify":
		flags := flag.NewFlagSet("verify", flag.ContinueOnError)
		flags.SetOutput(output)
		root := flags.String("root", "", "runtime directory")
		manifestPath := flags.String("manifest", "", "manifest path")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if *root == "" {
			return errors.New("verify requires -root")
		}
		if *manifestPath == "" {
			*manifestPath = filepath.Join(*root, "manifest.json")
		}
		verified, err := runtimebundle.VerifyManifest(*root, *manifestPath)
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(verified)
	case "checksums":
		flags := flag.NewFlagSet("checksums", flag.ContinueOnError)
		flags.SetOutput(output)
		path := flags.String("output", "SHA256SUMS", "checksum output")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		files := flags.Args()
		if len(files) == 0 {
			return errors.New("checksums requires at least one artifact")
		}
		sort.Strings(files)
		var lines strings.Builder
		for _, file := range files {
			digest, err := digestFile(file)
			if err != nil {
				return err
			}
			fmt.Fprintf(&lines, "%s  %s\n", digest, filepath.Base(file))
		}
		return os.WriteFile(*path, []byte(lines.String()), 0o644)
	case "scan-paths":
		flags := flag.NewFlagSet("scan-paths", flag.ContinueOnError)
		flags.SetOutput(output)
		root := flags.String("root", "", "artifact directory")
		needle := flags.String("needle", "", "forbidden absolute path")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if *root == "" || *needle == "" || !filepath.IsAbs(*needle) {
			return errors.New("scan-paths requires -root and an absolute -needle")
		}
		return scanForPath(*root, *needle)
	case "set-version":
		flags := flag.NewFlagSet("set-version", flag.ContinueOnError)
		flags.SetOutput(output)
		path := flags.String("file", "wails.json", "Wails configuration")
		version := flags.String("version", "", "release version")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if !releaseVersionRE.MatchString(*version) {
			return errors.New("set-version requires a semantic -version without a v prefix")
		}
		data, err := os.ReadFile(*path)
		if err != nil {
			return err
		}
		var config map[string]any
		if err := json.Unmarshal(data, &config); err != nil {
			return err
		}
		info, ok := config["info"].(map[string]any)
		if !ok {
			return errors.New("wails configuration has no info object")
		}
		// Native bundle/file versions must stay numeric even when the release
		// label is a valid semantic pre-release such as 1.2.3-rc.1.
		productVersion := strings.FieldsFunc(*version, func(r rune) bool { return r == '-' || r == '+' })[0]
		info["productVersion"] = productVersion
		updated, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(*path, append(updated, '\n'), 0o644)
	default:
		return fmt.Errorf("unknown nfcx-release command %q", arguments[0])
	}
}

func digestFile(path string) (string, error) {
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

func scanForPath(root, needle string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), needle) {
			return fmt.Errorf("artifact %s contains forbidden build path", path)
		}
		return nil
	})
}
