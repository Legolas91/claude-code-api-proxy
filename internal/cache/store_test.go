package cache

import (
	"sync"
	"testing"

	"github.com/claude-code-proxy/proxy/pkg/models"
)

func makeResp(text string) *models.ClaudeResponse {
	return &models.ClaudeResponse{
		ID:   "test-id",
		Type: "message",
		Role: "assistant",
		Content: []models.ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

func TestNewMemoryStore(t *testing.T) {
	s := NewMemoryStore(10)
	if s.Len() != 0 {
		t.Errorf("expected Len()=0, got %d", s.Len())
	}
}

func TestGetSet(t *testing.T) {
	s := NewMemoryStore(10)
	resp := makeResp("hello")
	s.Set("k1", resp)

	got := s.Get("k1")
	if got == nil {
		t.Fatal("expected cache hit, got nil")
	}
	if got.Content[0].Text != "hello" {
		t.Errorf("expected 'hello', got %q", got.Content[0].Text)
	}
}

func TestGetMiss(t *testing.T) {
	s := NewMemoryStore(10)
	if got := s.Get("missing"); got != nil {
		t.Errorf("expected nil on miss, got %v", got)
	}
}

func TestLRUEviction(t *testing.T) {
	s := NewMemoryStore(3)
	s.Set("k1", makeResp("1"))
	s.Set("k2", makeResp("2"))
	s.Set("k3", makeResp("3"))
	// k1 is now the LRU — adding k4 should evict it
	s.Set("k4", makeResp("4"))

	if s.Len() != 3 {
		t.Errorf("expected Len()=3, got %d", s.Len())
	}
	if s.Get("k1") != nil {
		t.Error("expected k1 to be evicted")
	}
	if s.Get("k4") == nil {
		t.Error("expected k4 to be present")
	}
}

func TestLRUPromotion(t *testing.T) {
	s := NewMemoryStore(3)
	s.Set("k1", makeResp("1"))
	s.Set("k2", makeResp("2"))
	s.Set("k3", makeResp("3"))
	// Promote k1 — now k2 is LRU
	s.Get("k1")
	// k2 should be evicted, k1 should survive
	s.Set("k4", makeResp("4"))

	if s.Get("k2") != nil {
		t.Error("expected k2 to be evicted after promotion of k1")
	}
	if s.Get("k1") == nil {
		t.Error("expected k1 to survive (was promoted)")
	}
}

func TestSetUpdateExisting(t *testing.T) {
	s := NewMemoryStore(3)
	s.Set("k1", makeResp("v1"))
	s.Set("k1", makeResp("v2"))

	if s.Len() != 1 {
		t.Errorf("expected Len()=1, got %d", s.Len())
	}
	got := s.Get("k1")
	if got == nil || got.Content[0].Text != "v2" {
		t.Errorf("expected updated value 'v2', got %v", got)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewMemoryStore(50)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "key"
			s.Set(key, makeResp("value"))
			s.Get(key)
		}(i)
	}
	wg.Wait()
	// No race condition — test passes if -race doesn't report issues
}

func TestComputeKey_Deterministic(t *testing.T) {
	req := models.ClaudeRequest{
		Model:     "claude-sonnet-4-20250514",
		Messages:  []models.ClaudeMessage{{Role: "user", Content: "hello"}},
		MaxTokens: 100,
	}
	k1 := ComputeKey(req)
	k2 := ComputeKey(req)
	if k1 != k2 {
		t.Errorf("same request produced different keys: %q vs %q", k1, k2)
	}
}

func TestComputeKey_DifferentInputs(t *testing.T) {
	req1 := models.ClaudeRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []models.ClaudeMessage{{Role: "user", Content: "hello"}},
	}
	req2 := models.ClaudeRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []models.ClaudeMessage{{Role: "user", Content: "world"}},
	}
	if ComputeKey(req1) == ComputeKey(req2) {
		t.Error("different messages produced the same key")
	}
}

func TestComputeKey_IgnoresStream(t *testing.T) {
	trueVal := true
	falseVal := false
	req1 := models.ClaudeRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []models.ClaudeMessage{{Role: "user", Content: "hello"}},
		Stream:   &trueVal,
	}
	req2 := models.ClaudeRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []models.ClaudeMessage{{Role: "user", Content: "hello"}},
		Stream:   &falseVal,
	}
	if ComputeKey(req1) != ComputeKey(req2) {
		t.Error("Stream field should not affect the cache key")
	}
}
