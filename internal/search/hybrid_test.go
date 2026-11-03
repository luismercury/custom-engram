package search

import (
	"context"
	"testing"

	"custom-engram/internal/store"
)

type mockEmbedder struct {
	vector []float32
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return m.vector, nil
}

func (m *mockEmbedder) ModelName() string {
	return "mock-model"
}

func (m *mockEmbedder) IsAvailable(ctx context.Context) bool {
	return true
}

func TestHybridSearch_Fusion(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("Store error: %v", err)
	}
	defer st.Close()

	proj, _ := st.GetOrCreateProject("test-hybrid", "C:/test")

	// Obs 1: Contains keyword "database"
	obs1 := &store.Observation{
		ProjectID: proj.ID,
		Title:     "Relational Database Schema",
		Content:   "SQLite database schema design.",
	}
	_ = st.SaveObservation(obs1)
	_ = st.SaveEmbedding(obs1.ID, "mock-model", []float32{1.0, 0.0, 0.0})

	// Obs 2: Does not contain keyword, but conceptually similar vector
	obs2 := &store.Observation{
		ProjectID: proj.ID,
		Title:     "Persistence Layer",
		Content:   "Storing structured records on disk.",
	}
	_ = st.SaveObservation(obs2)
	_ = st.SaveEmbedding(obs2.ID, "mock-model", []float32{0.9, 0.1, 0.0})

	mock := &mockEmbedder{vector: []float32{0.95, 0.05, 0.0}}

	// Search for "database"
	results, err := HybridSearch(context.Background(), st, mock, proj.ID, "database", 5)
	if err != nil {
		t.Fatalf("HybridSearch failed: %v", err)
	}

	if len(results) < 2 {
		t.Fatalf("Expected at least 2 results (1 from FTS, 1 from vector), got %d", len(results))
	}

	// Verify that obs1 has source 'hybrid' because it matched both FTS5 ("database") and high vector similarity
	foundHybrid := false
	for _, r := range results {
		if r.Observation.ID == obs1.ID && r.Source == "hybrid" {
			foundHybrid = true
			break
		}
	}
	if !foundHybrid {
		t.Errorf("Expected obs1 to have 'hybrid' source")
	}
}
