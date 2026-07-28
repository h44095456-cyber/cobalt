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

	found := false
	for _, res := range results {
		if res.Name == "json" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected package 'json' in search results")
	}
}
