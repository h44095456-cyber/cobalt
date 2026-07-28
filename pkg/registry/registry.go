package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PackageMetadata struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Authors     []string `json:"authors"`
}

type Registry struct {
	packages map[string]PackageMetadata
}

func New() *Registry {
	reg := &Registry{
		packages: make(map[string]PackageMetadata),
	}

	// Seed standard community package registry index
	reg.packages["http"] = PackageMetadata{
		Name:        "http",
		Version:     "1.2.0",
		Description: "High-performance multi-threaded HTTP/1.1 web server and client for Cobalt",
		Authors:     []string{"Cobalt Core Team"},
	}
	reg.packages["json"] = PackageMetadata{
		Name:        "json",
		Version:     "2.0.1",
		Description: "Recursive JSON parser, validator, and pretty formatter",
		Authors:     []string{"Cobalt Core Team"},
	}
	reg.packages["math"] = PackageMetadata{
		Name:        "math",
		Version:     "0.5.0",
		Description: "Advanced linear algebra and 3D vector graphics library",
		Authors:     []string{"Cobalt Math Working Group"},
	}
	reg.packages["sqlite"] = PackageMetadata{
		Name:        "sqlite",
		Version:     "3.40.0",
		Description: "Native FFI bindings for SQLite embedded relational database engine",
		Authors:     []string{"Database Contributors"},
	}
	reg.packages["regex"] = PackageMetadata{
		Name:        "regex",
		Version:     "1.0.4",
		Description: "High-speed regular expression matching engine",
		Authors:     []string{"Cobalt Core Team"},
	}

	return reg
}

func (r *Registry) Search(query string) []PackageMetadata {
	var results []PackageMetadata
	q := strings.ToLower(query)

	for _, pkg := range r.packages {
		if strings.Contains(strings.ToLower(pkg.Name), q) || strings.Contains(strings.ToLower(pkg.Description), q) {
			results = append(results, pkg)
		}
	}
	return results
}

func (r *Registry) PublishCurrentProject() (*PackageMetadata, error) {
	tomlPath := "cobalt.toml"
	if _, err := os.Stat(tomlPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no cobalt.toml found in current directory. Run 'cobalt init' first")
	}

	content, err := os.ReadFile(tomlPath)
	if err != nil {
		return nil, err
	}

	pkgName := "custom_pkg"
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "name =") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				pkgName = strings.Trim(strings.TrimSpace(parts[1]), "\"")
			}
		}
	}

	meta := PackageMetadata{
		Name:        pkgName,
		Version:     "0.1.0",
		Description: "Cobalt package published to registry",
		Authors:     []string{"Developer"},
	}

	regDir := filepath.Join(".cobalt", "registry")
	os.MkdirAll(regDir, 0755)

	data, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(regDir, pkgName+".json"), data, 0644)

	return &meta, nil
}
