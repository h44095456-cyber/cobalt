package pm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackageManagerInstallAndLock(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cobalt_pm_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := New(tmpDir)

	pkgSpec, err := mgr.InstallDependency("json", "2.0.1")
	if err != nil {
		t.Fatalf("InstallDependency failed: %v", err)
	}

	if pkgSpec.Name != "json" || pkgSpec.Version != "2.0.1" {
		t.Errorf("Unexpected pkgSpec: %+v", pkgSpec)
	}

	// Verify cobalt.toml exists
	tomlPath := filepath.Join(tmpDir, "cobalt.toml")
	if _, err := os.Stat(tomlPath); os.IsNotExist(err) {
		t.Fatalf("cobalt.toml was not generated")
	}

	// Verify cobalt.lock exists
	lockPath := filepath.Join(tmpDir, "cobalt.lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatalf("cobalt.lock was not generated")
	}

	// Load lockfile and verify
	lock, err := mgr.LoadLockfile()
	if err != nil {
		t.Fatalf("LoadLockfile failed: %v", err)
	}

	if len(lock.Packages) != 1 {
		t.Fatalf("Expected 1 package in lockfile, got %d", len(lock.Packages))
	}

	if lock.Packages[0].Name != "json" {
		t.Errorf("Expected package 'json', got '%s'", lock.Packages[0].Name)
	}

	// Verify lockfile integrity
	issues, err := mgr.VerifyLockfile()
	if err != nil {
		t.Fatalf("VerifyLockfile failed: %v", err)
	}
	if len(issues) > 0 {
		t.Errorf("VerifyLockfile reported unexpected issues: %v", issues)
	}
}
