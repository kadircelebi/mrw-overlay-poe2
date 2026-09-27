// Command packext builds the store packages of the browser extension from
// browser-extension/, which stays the source of truth (the app also copies
// it for "Load unpacked").
//
//	go run ./cmd/packext            → dist/extension/*.zip
//
// Chrome Web Store / Edge Add-ons: no "key" (the stores assign the ID), no
// Firefox settings, no background.scripts. Firefox AMO: no "key", no
// service_worker (Firefox runs background.scripts). Zip entries always use
// forward slashes; AMO rejects Windows-style paths.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	src := flag.String("src", "browser-extension", "extension source folder")
	out := flag.String("out", filepath.Join("dist", "extension"), "output folder")
	flag.Parse()
	log.SetFlags(0)

	raw, err := os.ReadFile(filepath.Join(*src, "manifest.json"))
	if err != nil {
		log.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		log.Fatal(err)
	}
	version, _ := manifest["version"].(string)
	if version == "" {
		log.Fatal("manifest has no version")
	}
	files, err := sourceFiles(*src)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, target := range []string{"chrome", "firefox"} {
		m, err := ForStore(manifest, target)
		if err != nil {
			log.Fatal(err)
		}
		name := filepath.Join(*out, fmt.Sprintf("mrw-overlay-bridge-%s-%s.zip", target, version))
		if err := writeZip(name, *src, files, m); err != nil {
			log.Fatal(err)
		}
		fmt.Println(name)
	}
}

// ForStore returns the manifest as one store wants it.
func ForStore(src map[string]any, target string) ([]byte, error) {
	m := clone(src)
	delete(m, "key")
	bg, _ := m["background"].(map[string]any)
	switch target {
	case "chrome":
		delete(m, "browser_specific_settings")
		if bg != nil {
			delete(bg, "scripts")
		}
	case "firefox":
		if bg != nil {
			delete(bg, "service_worker")
		}
	default:
		return nil, fmt.Errorf("unknown store %q", target)
	}
	return json.MarshalIndent(m, "", "  ")
}

func clone(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

// sourceFiles lists what goes into the package: everything but the manifest
// (written per store) and dot files.
func sourceFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && p != root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel != "manifest.json" {
			files = append(files, rel)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func writeZip(name, root string, files []string, manifest []byte) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// A fixed time keeps the package byte-identical between builds.
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	add := func(entry string, data []byte) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: path.Clean(entry), Method: zip.Deflate, Modified: stamp})
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := add("manifest.json", manifest); err != nil {
		return err
	}
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			return err
		}
		if err := add(f, data); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(name, buf.Bytes(), 0o644)
}
