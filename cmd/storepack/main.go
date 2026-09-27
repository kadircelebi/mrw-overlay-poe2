// Command storepack builds the Microsoft Store package (MSIX).
//
//	go run ./cmd/storepack            → dist/store/MrWOverlay_<version>_x64.msix
//
// It builds the exe with the "store" tag (no GitHub self-updater), lays out
// the package with the tile images from build/windows/msix/Assets, renders
// AppxManifest.xml from build/windows/msix/identity.json, indexes the images
// (resources.pri, so Windows picks the right size) and packs with makeappx.
// The package is left unsigned: the Store signs it. Needs the Windows SDK
// (makeappx.exe, makepri.exe) and wails3 on PATH or in GOPATH/bin.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"text/template"
)

type identity struct {
	IdentityName         string `json:"identityName"`
	Publisher            string `json:"publisher"`
	PublisherDisplayName string `json:"publisherDisplayName"`
	DisplayName          string `json:"displayName"`
	Description          string `json:"description"`
	Version              string `json:"-"`
}

func main() {
	out := flag.String("out", filepath.Join("dist", "store"), "output folder")
	exe := flag.String("exe", "", "use this already built store exe instead of building one")
	flag.Parse()
	log.SetFlags(0)

	if err := run(*out, *exe); err != nil {
		log.Fatal(err)
	}
}

func run(out, prebuilt string) error {
	id, err := loadIdentity()
	if err != nil {
		return err
	}
	if strings.Contains(id.IdentityName, "PLACEHOLDER") || strings.Contains(id.Publisher, "00000000-") {
		log.Println("warning: identity.json still has placeholders; the package installs locally but the Store will refuse it")
	}
	layout := filepath.Join(out, "layout")
	if err := os.RemoveAll(layout); err != nil {
		return err
	}
	if err := os.MkdirAll(layout, 0o755); err != nil {
		return err
	}

	target := filepath.Join(layout, "poe2filter.exe")
	if prebuilt != "" {
		if err := copyFile(prebuilt, target); err != nil {
			return err
		}
	} else if err := buildExe(target, strings.TrimSuffix(id.Version, ".0")); err != nil {
		return err
	}
	if err := copyDir(filepath.Join("build", "windows", "msix", "Assets"), filepath.Join(layout, "Assets")); err != nil {
		return err
	}
	if err := writeManifest(id, filepath.Join(layout, "AppxManifest.xml")); err != nil {
		return err
	}
	if err := makePRI(layout, out); err != nil {
		return err
	}
	pkg := filepath.Join(out, fmt.Sprintf("MrWOverlay_%s_x64.msix", id.Version))
	makeappx, err := sdkTool("makeappx.exe")
	if err != nil {
		return err
	}
	if err := runCmd(makeappx, "pack", "/o", "/d", layout, "/p", pkg); err != nil {
		return err
	}
	fmt.Println(pkg)
	return nil
}

var versionRE = regexp.MustCompile(`(?m)^\s*version:\s*"?(\d+)\.(\d+)\.(\d+)"?`)

// loadIdentity reads identity.json and takes the version from build/config.yml
// (the same file TestVersionIsConsistent checks). The Store wants a.b.c.0.
func loadIdentity() (identity, error) {
	var id identity
	b, err := os.ReadFile(filepath.Join("build", "windows", "msix", "identity.json"))
	if err != nil {
		return id, err
	}
	if err := json.Unmarshal(b, &id); err != nil {
		return id, err
	}
	cfg, err := os.ReadFile(filepath.Join("build", "config.yml"))
	if err != nil {
		return id, err
	}
	m := versionRE.FindStringSubmatch(string(cfg))
	if m == nil {
		return id, errors.New("no version in build/config.yml")
	}
	id.Version = fmt.Sprintf("%s.%s.%s.0", m[1], m[2], m[3])
	return id, nil
}

func writeManifest(id identity, path string) error {
	tpl, err := template.ParseFiles(filepath.Join("build", "windows", "msix", "AppxManifest.xml.tmpl"))
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return tpl.Execute(f, xmlEscaped(id))
}

func xmlEscaped(id identity) identity {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	id.IdentityName, id.Publisher = r.Replace(id.IdentityName), r.Replace(id.Publisher)
	id.PublisherDisplayName, id.DisplayName = r.Replace(id.PublisherDisplayName), r.Replace(id.DisplayName)
	id.Description = r.Replace(id.Description)
	return id
}

// buildExe mirrors .github/workflows/release.yml, plus the store tag.
func buildExe(target, version string) error {
	// The exe embeds frontend/dist, so a stale dist ships an old interface.
	npm := exec.Command("npm", "run", "build")
	if runtime.GOOS == "windows" {
		npm = exec.Command("cmd", "/c", "npm", "run", "build")
	}
	npm.Dir = "frontend"
	npm.Stdout, npm.Stderr = os.Stderr, os.Stderr
	if err := npm.Run(); err != nil {
		return fmt.Errorf("npm run build: %w", err)
	}
	if _, err := os.Stat(filepath.Join("frontend", "dist", "index.html")); err != nil {
		return errors.New("frontend/dist is missing after npm run build")
	}
	wails, err := exec.LookPath("wails3")
	if err != nil {
		gopath, _ := exec.Command("go", "env", "GOPATH").Output()
		wails = filepath.Join(strings.TrimSpace(string(gopath)), "bin", "wails3.exe")
	}
	syso, _ := filepath.Abs("wails_windows_amd64.syso")
	defer os.Remove(syso)
	gen := exec.Command(wails, "generate", "syso", "-arch", "amd64",
		"-icon", filepath.Join("windows", "icon.ico"), "-manifest", filepath.Join("windows", "wails.exe.manifest"),
		"-info", filepath.Join("windows", "info.json"), "-out", syso)
	gen.Dir = "build"
	gen.Stdout, gen.Stderr = os.Stderr, os.Stderr
	if err := gen.Run(); err != nil {
		return fmt.Errorf("wails3 generate syso: %w", err)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	build := exec.Command("go", "build", "-tags", "production,store", "-trimpath",
		"-ldflags", "-w -s -H windowsgui -X main.version="+version, "-o", abs, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=windows", "GOARCH=amd64")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	return build.Run()
}

// makePRI indexes the images so Windows can pick the scaled/target-size
// variants (Square44x44Logo.targetsize-*); without it only the plain names
// are used.
func makePRI(layout, work string) error {
	makepri, err := sdkTool("makepri.exe")
	if err != nil {
		return err
	}
	cfg := filepath.Join(work, "priconfig.xml")
	if err := runCmd(makepri, "createconfig", "/cf", cfg, "/dq", "en-US", "/pv", "10.0.0", "/o"); err != nil {
		return err
	}
	return runCmd(makepri, "new", "/pr", layout, "/cf", cfg,
		"/mn", filepath.Join(layout, "AppxManifest.xml"), "/of", filepath.Join(layout, "resources.pri"), "/o")
}

// sdkTool finds the newest x64 copy of a Windows SDK tool.
func sdkTool(name string) (string, error) {
	root := filepath.Join(os.Getenv("ProgramFiles(x86)"), "Windows Kits", "10", "bin")
	matches, _ := filepath.Glob(filepath.Join(root, "10.*", "x64", name))
	if len(matches) == 0 {
		return "", fmt.Errorf("%s not found under %s: install the Windows SDK", name, root)
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	outb, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s", filepath.Base(name), args[0], err, outb)
	}
	return nil
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
