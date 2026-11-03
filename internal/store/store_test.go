package store

import (
	"testing"
	"time"
)

func TestStore_InMemory(t *testing.T) {
	st, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to init in-memory store: %v", err)
	}
	defer st.Close()

	// 1. Projects
	proj, err := st.GetOrCreateProject("test-project", "C:/test")
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	if proj.Name != "test-project" {
		t.Errorf("Expected project name 'test-project', got '%s'", proj.Name)
	}

	// 2. Sessions
	sess, err := st.StartSession(proj.ID, "Test goal")
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}
	if sess.Goal != "Test goal" {
		t.Errorf("Unexpected session goal: %s", sess.Goal)
	}

	// 3. Observations and FTS5 triggers
	obs := &Observation{
		ProjectID: proj.ID,
		SessionID: &sess.ID,
		Type:      "architecture",
		Scope:     "project",
		Title:     "SQLite FTS5 Architecture",
		Content:   "We are adopting modernc.org/sqlite for zero CGO FTS5 persistent memory.",
		TopicKey:  "arch/storage",
	}
	if err := st.SaveObservation(obs); err != nil {
		t.Fatalf("Failed to save observation: %v", err)
	}

	// Check retrieval by ID
	fetched, err := st.GetObservation(obs.ID)
	if err != nil {
		t.Fatalf("Failed to get observation: %v", err)
	}
	if fetched.Title != obs.Title {
		t.Errorf("Expected title '%s', got '%s'", obs.Title, fetched.Title)
	}

	// 4. FTS5 Search
	results, err := st.SearchFTS5(proj.ID, "modernc", 10)
	if err != nil {
		t.Fatalf("FTS5 search error: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("Expected at least 1 FTS5 result for 'modernc', got 0")
	}
	if results[0].Observation.ID != obs.ID {
		t.Errorf("Expected matched obs ID %s, got %s", obs.ID, results[0].Observation.ID)
	}

	// 5. Embeddings and Cosine Similarity
	vec1 := []float32{1.0, 0.0, 0.0}
	vec2 := []float32{0.8, 0.6, 0.0}
	sim := CosineSimilarity(vec1, vec2)
	if sim < 0.79 || sim > 0.81 {
		t.Errorf("Expected cosine similarity ~0.8, got %f", sim)
	}

	if err := st.SaveEmbedding(obs.ID, "test-model", vec1); err != nil {
		t.Fatalf("Failed to save embedding: %v", err)
	}

	embs, err := st.GetEmbeddingsForProject(proj.ID)
	if err != nil {
		t.Fatalf("Failed to get embeddings: %v", err)
	}
	if len(embs) != 1 {
		t.Fatalf("Expected 1 embedding, got %d", len(embs))
	}

	// 6. End session
	if err := st.EndSession(sess.ID, "Session completed successfully"); err != nil {
		t.Fatalf("Failed to end session: %v", err)
	}
}

func TestStore_ArchiveAndPrune(t *testing.T) {
	st, err := New(":memory:")
	if err != nil {
		t.Fatalf("Failed to init in-memory store: %v", err)
	}
	defer st.Close()

	proj, err := st.GetOrCreateProject("prune-test", "C:/test")
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create 4 observations:
	// 1: architecture (old, but protected type)
	// 2: discovery (old, perishable)
	// 3: discovery (old, but pinned -> protected)
	// 4: discovery (recent)

	now := time.Now().UTC()
	oldTime := now.Add(-100 * 24 * time.Hour)

	obsArch := &Observation{
		ProjectID: proj.ID,
		Type:      "architecture",
		Title:     "Old Architecture",
		Content:   "Database architecture details",
	}
	if err := st.SaveObservation(obsArch); err != nil {
		t.Fatal(err)
	}
	// Manually backdate updated_at
	_, _ = st.DB().Exec(`UPDATE observations SET updated_at = ? WHERE id = ?`, oldTime.Format(time.RFC3339Nano), obsArch.ID)

	obsPerishable := &Observation{
		ProjectID: proj.ID,
		Type:      "discovery",
		Title:     "Old Temporary Discovery",
		Content:   "Temporary workaround for old bug",
	}
	if err := st.SaveObservation(obsPerishable); err != nil {
		t.Fatal(err)
	}
	_, _ = st.DB().Exec(`UPDATE observations SET updated_at = ? WHERE id = ?`, oldTime.Format(time.RFC3339Nano), obsPerishable.ID)

	obsPinned := &Observation{
		ProjectID: proj.ID,
		Type:      "discovery",
		Title:     "Old Pinned Discovery",
		Content:   "Crucial persistent discovery",
		IsPinned:  true,
	}
	if err := st.SaveObservation(obsPinned); err != nil {
		t.Fatal(err)
	}
	_, _ = st.DB().Exec(`UPDATE observations SET updated_at = ? WHERE id = ?`, oldTime.Format(time.RFC3339Nano), obsPinned.ID)

	obsRecent := &Observation{
		ProjectID: proj.ID,
		Type:      "discovery",
		Title:     "Recent Discovery",
		Content:   "Fresh insights",
	}
	if err := st.SaveObservation(obsRecent); err != nil {
		t.Fatal(err)
	}

	// Test 1: DryRun should find exactly 1 candidate (obsPerishable) without modifying DB
	dryRes, err := st.Prune(PruneOptions{
		ProjectID: proj.ID,
		OlderThan: 30 * 24 * time.Hour,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("Prune dry-run failed: %v", err)
	}
	if dryRes.MatchedCount != 1 {
		t.Fatalf("Expected 1 candidate in dry-run, got %d", dryRes.MatchedCount)
	}
	if dryRes.Observations[0].ID != obsPerishable.ID {
		t.Errorf("Expected candidate %s, got %s", obsPerishable.ID, dryRes.Observations[0].ID)
	}

	// Verify obsPerishable is NOT archived yet
	checkObs, _ := st.GetObservation(obsPerishable.ID)
	if checkObs.IsArchived {
		t.Errorf("Observation should not be archived during dry-run")
	}

	// Test 2: Real Prune (Soft Delete / Archive)
	realRes, err := st.Prune(PruneOptions{
		ProjectID: proj.ID,
		OlderThan: 30 * 24 * time.Hour,
		DryRun:    false,
	})
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if realRes.AffectedCount != 1 {
		t.Fatalf("Expected 1 affected observation, got %d", realRes.AffectedCount)
	}

	// Verify obsPerishable is now archived
	checkObs, _ = st.GetObservation(obsPerishable.ID)
	if !checkObs.IsArchived {
		t.Errorf("Observation should be archived after real prune")
	}

	// Verify active observations list: should contain arch, pinned, recent (NOT perishable)
	activeList, err := st.ListObservations(proj.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(activeList) != 3 {
		t.Fatalf("Expected 3 active observations, got %d", len(activeList))
	}
	for _, o := range activeList {
		if o.ID == obsPerishable.ID {
			t.Errorf("Archived observation %s should not appear in ListObservations", o.ID)
		}
	}

	// Verify SearchFTS5 doesn't return archived observation
	searchRes, err := st.SearchFTS5(proj.ID, "workaround", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(searchRes) != 0 {
		t.Errorf("Archived observation should not be returned by SearchFTS5")
	}

	// Test 3: ListArchivedObservations
	archList, err := st.ListArchivedObservations(proj.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(archList) != 1 || archList[0].ID != obsPerishable.ID {
		t.Fatalf("Expected obsPerishable in ListArchivedObservations")
	}

	// Test 4: Unarchive
	if err := st.ArchiveObservation(obsPerishable.ID, false); err != nil {
		t.Fatalf("Failed to unarchive: %v", err)
	}
	checkObs, _ = st.GetObservation(obsPerishable.ID)
	if checkObs.IsArchived {
		t.Errorf("Observation should no longer be archived")
	}

	// Test 5: Hard delete via prune
	// Re-backdate obsPerishable
	_, _ = st.DB().Exec(`UPDATE observations SET updated_at = ? WHERE id = ?`, oldTime.Format(time.RFC3339Nano), obsPerishable.ID)
	hardRes, err := st.Prune(PruneOptions{
		ProjectID:  proj.ID,
		OlderThan:  30 * 24 * time.Hour,
		HardDelete: true,
		DryRun:     false,
	})
	if err != nil {
		t.Fatalf("Hard prune failed: %v", err)
	}
	if hardRes.AffectedCount != 1 {
		t.Fatalf("Expected 1 hard-deleted observation, got %d", hardRes.AffectedCount)
	}
	if _, err := st.GetObservation(obsPerishable.ID); err == nil {
		t.Errorf("Observation should be deleted from DB")
	}
}
