package index

import (
	"context"
	"testing"

	"backend/pkg/models"
)

func makeRecords() []models.Record {
	return []models.Record{
		{ID: 1, Message: "disk failure detected on server", AppName: "storage", Hostname: "host1"},
		{ID: 2, Message: "disk space running low warning", AppName: "monitor", Hostname: "host2"},
		{ID: 3, Message: "authentication failed login attempt", AppName: "auth", Hostname: "host3"},
	}
}

// --- Build tests ---

func TestBuild_IndexesTermsAndDocs(t *testing.T) {
	idx := New()
	defer idx.Stop()

	records := makeRecords()
	if err := idx.Build(context.Background(), records, 2); err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	stats := idx.Stats()
	if stats["total_docs"] != len(records) {
		t.Errorf("expected %d total_docs, got %d", len(records), stats["total_docs"])
	}
	if stats["total_terms"] == 0 {
		t.Error("expected non-zero total_terms after build")
	}

	// "disk" should appear in the inverted index (matches docs 1 and 2)
	posts, ok := idx.inverted["disk"]
	if !ok || len(posts) == 0 {
		t.Error("expected 'disk' to be indexed")
	}
	if idx.docFreq["disk"] != 2 {
		t.Errorf("expected docFreq['disk']=2, got %d", idx.docFreq["disk"])
	}
}

func TestBuild_ContextCancellation(t *testing.T) {
	idx := New()
	defer idx.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before Build starts

	err := idx.Build(ctx, makeRecords(), 2)
	if err == nil {
		t.Error("expected error when context is cancelled, got nil")
	}
}

// --- Search tests ---

func TestSearch_RanksRelevantResultsFirst(t *testing.T) {
	idx := New()
	defer idx.Stop()

	if err := idx.Build(context.Background(), makeRecords(), 2); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	results, elapsed := idx.Search("disk failure")
	if elapsed < 0 {
		t.Error("elapsed time should be non-negative")
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'disk failure'")
	}
	// doc 1 mentions "disk failure" so it should rank highest
	if results[0].Record.ID != 1 {
		t.Errorf("expected doc ID 1 to rank first, got %d", results[0].Record.ID)
	}
	// scores must be in descending order
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted by score desc at index %d", i)
		}
	}
}

func TestSearch_EmptyQueryReturnsNoResults(t *testing.T) {
	idx := New()
	defer idx.Stop()

	if err := idx.Build(context.Background(), makeRecords(), 2); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// stopwords-only query tokenizes to nothing
	results, _ := idx.Search("the and or")
	if len(results) != 0 {
		t.Errorf("expected 0 results for stopword-only query, got %d", len(results))
	}

	results, _ = idx.Search("")
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}
