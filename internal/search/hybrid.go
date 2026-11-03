package search

import (
	"context"
	"sort"

	"custom-engram/internal/store"
)

type vectorCandidate struct {
	id    string
	score float64
}

// HybridSearch performs combined lexical (FTS5) and semantic (Embeddings) search.
func HybridSearch(ctx context.Context, st *store.Store, embedder Embedder, projectID, query string, limit int) ([]store.SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// 1. Fetch lexical FTS5 results
	ftsResults, err := st.SearchFTS5(projectID, query, limit*2)
	if err != nil {
		return nil, err
	}

	// If no embedder or embeddings not available, return FTS5 directly
	if embedder == nil || !embedder.IsAvailable(ctx) {
		if len(ftsResults) > limit {
			return ftsResults[:limit], nil
		}
		return ftsResults, nil
	}

	// 2. Compute query embedding
	queryVec, err := embedder.Embed(ctx, query)
	if err != nil || len(queryVec) == 0 {
		if len(ftsResults) > limit {
			return ftsResults[:limit], nil
		}
		return ftsResults, nil
	}

	// 3. Compare with project embeddings
	allEmbs, err := st.GetEmbeddingsForProject(projectID)
	if err != nil || len(allEmbs) == 0 {
		if len(ftsResults) > limit {
			return ftsResults[:limit], nil
		}
		return ftsResults, nil
	}

	var vectorRankings []vectorCandidate
	for obsID, vec := range allEmbs {
		sim := store.CosineSimilarity(queryVec, vec)
		if sim > 0.25 { // Minimum semantic threshold
			vectorRankings = append(vectorRankings, vectorCandidate{
				id:    obsID,
				score: sim,
			})
		}
	}

	// Sort vector candidates descending by similarity
	sort.Slice(vectorRankings, func(i, j int) bool {
		return vectorRankings[i].score > vectorRankings[j].score
	})

	// 4. Reciprocal Rank Fusion (RRF)
	const k = 60.0
	rrfScores := make(map[string]float64)
	sourceMap := make(map[string]string)
	obsMap := make(map[string]store.Observation)

	// Add FTS5 ranks
	for rank, r := range ftsResults {
		id := r.Observation.ID
		rrfScores[id] += 1.0 / (k + float64(rank+1))
		obsMap[id] = r.Observation
		sourceMap[id] = "fts5"
	}

	// Add Vector ranks
	var missingIDs []string
	for rank, v := range vectorRankings {
		rrfScores[v.id] += 1.0 / (k + float64(rank+1))
		if _, exists := obsMap[v.id]; !exists {
			missingIDs = append(missingIDs, v.id)
			sourceMap[v.id] = "vector"
		} else {
			sourceMap[v.id] = "hybrid"
		}
	}

	// Retrieve observations that matched via vector but not FTS5
	if len(missingIDs) > 0 {
		missingObs, err := st.GetObservationsByIDs(missingIDs)
		if err == nil {
			for _, o := range missingObs {
				obsMap[o.ID] = o
			}
		}
	}

	// Build final merged list
	type fusedItem struct {
		obs    store.Observation
		score  float64
		source string
	}

	var fused []fusedItem
	for id, score := range rrfScores {
		obs, found := obsMap[id]
		if found {
			fused = append(fused, fusedItem{
				obs:    obs,
				score:  score,
				source: sourceMap[id],
			})
		}
	}

	sort.Slice(fused, func(i, j int) bool {
		return fused[i].score > fused[j].score
	})

	var finalResults []store.SearchResult
	for i := 0; i < len(fused) && i < limit; i++ {
		finalResults = append(finalResults, store.SearchResult{
			Observation: fused[i].obs,
			Score:       fused[i].score,
			Source:      fused[i].source,
		})
	}

	return finalResults, nil
}
