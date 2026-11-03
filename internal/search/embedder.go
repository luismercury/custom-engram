package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	ModelName() string
	IsAvailable(ctx context.Context) bool
}

// OllamaEmbedder connects to a local Ollama instance.
type OllamaEmbedder struct {
	BaseURL    string
	Model      string
	httpClient *http.Client
}

func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	if baseURL == "" {
		baseURL = os.Getenv("OLLAMA_HOST")
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if model == "" {
		model = os.Getenv("ENGRAM_EMBED_MODEL")
		if model == "" {
			model = "nomic-embed-text"
		}
	}

	return &OllamaEmbedder{
		BaseURL: baseURL,
		Model:   model,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (o *OllamaEmbedder) ModelName() string {
	return o.Model
}

func (o *OllamaEmbedder) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", o.BaseURL+"/api/version", nil)
	if err != nil {
		return false
	}
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

func (o *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody, err := json.Marshal(ollamaEmbedRequest{
		Model:  o.Model,
		Prompt: text,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.BaseURL+"/api/embeddings", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embed error: status %d", resp.StatusCode)
	}

	var res ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return res.Embedding, nil
}

// NoopEmbedder is used when vector embeddings are not configured or available.
type NoopEmbedder struct{}

func (n *NoopEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

func (n *NoopEmbedder) ModelName() string {
	return "none"
}

func (n *NoopEmbedder) IsAvailable(ctx context.Context) bool {
	return false
}

// DetectEmbedder detects if a local embedder (like Ollama) is active, otherwise falls back gracefully.
func DetectEmbedder(ctx context.Context) Embedder {
	ollama := NewOllamaEmbedder("", "")
	checkCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	if ollama.IsAvailable(checkCtx) {
		return ollama
	}
	return &NoopEmbedder{}
}
