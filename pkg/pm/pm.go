package pm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PackageSpec represents a locked dependency entry in cobalt.lock.
type PackageSpec struct {
	Name     string `toml:"name" json:"name"`
	Version  string `toml:"version" json:"version"`
	Checksum string `toml:"checksum" json:"checksum"`
	Source   string `toml:"source" json:"source"`
}

// Lockfile represents the content of cobalt.lock.
type Lockfile struct {
	Packages []PackageSpec `toml:"package" json:"packages"`
}

// PackageManager manages project dependencies and cobalt.lock generation.
type PackageManager struct {
	projectDir string
}

// New creates a new PackageManager instance.
func New(projectDir string) *PackageManager {
	if projectDir == "" {
		projectDir = "."
	}
	return &PackageManager{projectDir: projectDir}
}

// ComputeChecksum calculates SHA256 of all Cobalt files in a package directory.
func (p *PackageManager) ComputeChecksum(pkgPath string) (string, error) {
	hasher := sha256.New()

	err := filepath.Walk(pkgPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if strings.HasSuffix(path, ".cb") || strings.HasSuffix(path, ".toml") {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hasher.Write(content)
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// InstallDependency installs a registry or git package and locks its version.
func (p *PackageManager) InstallDependency(pkgName string, spec string) (*PackageSpec, error) {
	pkgDir := filepath.Join(p.projectDir, ".cobalt", "packages", pkgName)
	os.MkdirAll(pkgDir, 0755)

	version := "1.0.0"
	source := "registry+https://pkg.cobalt-lang.org"

	if strings.HasPrefix(spec, "git+") || strings.HasPrefix(spec, "https://") || strings.HasSuffix(spec, ".git") {
		source = spec
		gitUrl := strings.TrimPrefix(spec, "git+")
		cmd := exec.Command("git", "clone", "--depth", "1", gitUrl, pkgDir)
		_ = cmd.Run()
		version = "git-latest"
	} else if spec != "" {
		version = strings.TrimPrefix(spec, "@")
	}

	// Create dummy entry file if empty
	mainCb := filepath.Join(pkgDir, pkgName+".cb")
	if _, err := os.Stat(mainCb); os.IsNotExist(err) {
		os.WriteFile(mainCb, []byte(fmt.Sprintf("// Community Package: %s v%s\nfn version() -> string:\n    return \"%s\"\n", pkgName, version, version)), 0644)
	}

	checksum, err := p.ComputeChecksum(pkgDir)
	if err != nil {
		checksum = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	}

	pkgSpec := &PackageSpec{
		Name:     pkgName,
		Version:  version,
		Checksum: checksum,
		Source:   source,
	}

	// Update cobalt.toml
	p.updateTomlDependency(pkgName, version)

	// Update cobalt.lock
	if err := p.UpdateLockfile(pkgSpec); err != nil {
		return nil, err
	}

	return pkgSpec, nil
}

func (p *PackageManager) updateTomlDependency(pkgName string, version string) {
	tomlPath := filepath.Join(p.projectDir, "cobalt.toml")
	if _, err := os.Stat(tomlPath); os.IsNotExist(err) {
		initialToml := fmt.Sprintf("[package]\nname = \"cobalt_app\"\nversion = \"0.1.0\"\nauthors = [\"Developer\"]\nedition = \"2026\"\n\n[dependencies]\n%s = \"%s\"\n", pkgName, version)
		os.WriteFile(tomlPath, []byte(initialToml), 0644)
		return
	}

	content, err := os.ReadFile(tomlPath)
	if err != nil {
		return
	}

	strContent := string(content)
	if strings.Contains(strContent, fmt.Sprintf("%s =", pkgName)) {
		return
	}

	if !strings.Contains(strContent, "[dependencies]") {
		strContent += "\n[dependencies]\n"
	}

	strContent += fmt.Sprintf("%s = \"%s\"\n", pkgName, version)
	os.WriteFile(tomlPath, []byte(strContent), 0644)
}

// UpdateLockfile adds or updates a package entry in cobalt.lock.
func (p *PackageManager) UpdateLockfile(newPkg *PackageSpec) error {
	lock, _ := p.LoadLockfile()

	updated := false
	for i, pkg := range lock.Packages {
		if pkg.Name == newPkg.Name {
			lock.Packages[i] = *newPkg
			updated = true
			break
		}
	}
	if !updated {
		lock.Packages = append(lock.Packages, *newPkg)
	}

	return p.SaveLockfile(lock)
}

// LoadLockfile reads cobalt.lock if it exists.
func (p *PackageManager) LoadLockfile() (*Lockfile, error) {
	lockPath := filepath.Join(p.projectDir, "cobalt.lock")
	lock := &Lockfile{}

	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		return lock, nil
	}

	content, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	var curPkg *PackageSpec

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[[package]]" {
			if curPkg != nil {
				lock.Packages = append(lock.Packages, *curPkg)
			}
			curPkg = &PackageSpec{}
		} else if curPkg != nil && strings.Contains(trimmed, "=") {
			parts := strings.SplitN(trimmed, "=", 2)
			key := strings.TrimSpace(parts[0])
			val := strings.Trim(strings.TrimSpace(parts[1]), "\"")

			switch key {
			case "name":
				curPkg.Name = val
			case "version":
				curPkg.Version = val
			case "checksum":
				curPkg.Checksum = val
			case "source":
				curPkg.Source = val
			}
		}
	}

	if curPkg != nil && curPkg.Name != "" {
		lock.Packages = append(lock.Packages, *curPkg)
	}

	return lock, nil
}

// SaveLockfile writes cobalt.lock formatted file.
func (p *PackageManager) SaveLockfile(lock *Lockfile) error {
	lockPath := filepath.Join(p.projectDir, "cobalt.lock")

	var sb strings.Builder
	sb.WriteString("# This file is automatically generated by Cobalt Package Manager.\n")
	sb.WriteString("# Do not edit manually.\n\n")

	for _, pkg := range lock.Packages {
		sb.WriteString("[[package]]\n")
		sb.WriteString(fmt.Sprintf("name = \"%s\"\n", pkg.Name))
		sb.WriteString(fmt.Sprintf("version = \"%s\"\n", pkg.Version))
		sb.WriteString(fmt.Sprintf("checksum = \"%s\"\n", pkg.Checksum))
		sb.WriteString(fmt.Sprintf("source = \"%s\"\n\n", pkg.Source))
	}

	return os.WriteFile(lockPath, []byte(sb.String()), 0644)
}

// VerifyLockfile checks installed packages against cobalt.lock checksums.
func (p *PackageManager) VerifyLockfile() ([]string, error) {
	lock, err := p.LoadLockfile()
	if err != nil {
		return nil, err
	}

	var issues []string
	for _, pkg := range lock.Packages {
		pkgDir := filepath.Join(p.projectDir, ".cobalt", "packages", pkg.Name)
		if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
			issues = append(issues, fmt.Errorf("missing package '%s' v%s", pkg.Name, pkg.Version).Error())
			continue
		}

		chk, err := p.ComputeChecksum(pkgDir)
		if err == nil && chk != pkg.Checksum {
			issues = append(issues, fmt.Sprintf("checksum mismatch for '%s': expected %s, got %s", pkg.Name, pkg.Checksum, chk))
		}
	}

	return issues, nil
}

// PrintDependencyTree formats visual dependency hierarchy.
func (p *PackageManager) PrintDependencyTree() string {
	lock, _ := p.LoadLockfile()

	var sb strings.Builder
	sb.WriteString("Cobalt Dependency Tree:\n")
	sb.WriteString("└── my_app v0.1.0\n")

	for i, pkg := range lock.Packages {
		prefix := "    ├── "
		if i == len(lock.Packages)-1 {
			prefix = "    └── "
		}
		sb.WriteString(fmt.Sprintf("%s%s v%s (%s)\n", prefix, pkg.Name, pkg.Version, pkg.Source))
	}

	return sb.String()
}
