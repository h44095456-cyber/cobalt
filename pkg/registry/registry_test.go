package registry

import (
	"testing"
)

func TestRegistrySearch(t *testing.T) {
	reg := New()
	results := reg.Search("json")

	if len(results) == 0 {
		t.Fatalf("expected package 'json' in registry search results")
	}

	if results[0].Name != "json" {
		t.Errorf("expected package name 'json', got %s", results[0].Name)
	}
}
