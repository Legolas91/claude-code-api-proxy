// Package cache provides a response cache for the proxy.
// The Store interface allows swapping the backend (in-memory, Redis, etc.)
// without changing the handler code.
package cache

import (
	"container/list"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/claude-code-proxy/proxy/pkg/models"
)

// Store is the response cache interface. Implementations must be safe for concurrent use.
type Store interface {
	// Get returns a cached ClaudeResponse for the given key, or nil if not found.
	Get(key string) *models.ClaudeResponse
	// Set stores a response under the given key, evicting the LRU entry if at capacity.
	Set(key string, resp *models.ClaudeResponse)
	// Len returns the number of entries currently in the cache.
	Len() int
}

// entry holds the key and value stored in the LRU list.
type entry struct {
	key  string
	resp *models.ClaudeResponse
}

// MemoryStore is an in-memory LRU cache. It is lost on restart (by design).
type MemoryStore struct {
	mu      sync.Mutex
	maxSize int
	items   map[string]*list.Element
	order   *list.List // front = most recently used
}

// NewMemoryStore creates a new MemoryStore with the given maximum number of entries.
func NewMemoryStore(maxSize int) *MemoryStore {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &MemoryStore{
		maxSize: maxSize,
		items:   make(map[string]*list.Element),
		order:   list.New(),
	}
}

// Get returns the cached response for key, or nil on a miss.
// A hit promotes the entry to the front of the LRU list.
func (m *MemoryStore) Get(key string) *models.ClaudeResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	el, ok := m.items[key]
	if !ok {
		return nil
	}
	m.order.MoveToFront(el)
	return el.Value.(*entry).resp
}

// Set stores resp under key. If the cache is at capacity, the least recently
// used entry is evicted first.
func (m *MemoryStore) Set(key string, resp *models.ClaudeResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update existing entry
	if el, ok := m.items[key]; ok {
		el.Value.(*entry).resp = resp
		m.order.MoveToFront(el)
		return
	}

	// Evict LRU if at capacity
	if len(m.items) >= m.maxSize {
		back := m.order.Back()
		if back != nil {
			m.order.Remove(back)
			delete(m.items, back.Value.(*entry).key)
		}
	}

	el := m.order.PushFront(&entry{key: key, resp: resp})
	m.items[key] = el
}

// Len returns the number of cached entries.
func (m *MemoryStore) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

// cacheKeyFields holds the fields of a ClaudeRequest that affect the response.
// Stream is intentionally excluded: a cached response is valid for both
// streaming and non-streaming callers (only non-streaming responses are stored).
type cacheKeyFields struct {
	Model         string                   `json:"model"`
	System        interface{}              `json:"system,omitempty"`
	Messages      []models.ClaudeMessage   `json:"messages"`
	Tools         []models.Tool            `json:"tools,omitempty"`
	ToolChoice    interface{}              `json:"tool_choice,omitempty"`
	Temperature   *float64                 `json:"temperature,omitempty"`
	MaxTokens     int                      `json:"max_tokens"`
	TopP          *float64                 `json:"top_p,omitempty"`
	StopSequences []string                 `json:"stop_sequences,omitempty"`
}

// ComputeKey returns a deterministic SHA-256 hex digest for the given request.
// Only fields that affect the response content are included.
func ComputeKey(req models.ClaudeRequest) string {
	fields := cacheKeyFields{
		Model:         req.Model,
		System:        req.System,
		Messages:      req.Messages,
		Tools:         req.Tools,
		ToolChoice:    req.ToolChoice,
		Temperature:   req.Temperature,
		MaxTokens:     req.MaxTokens,
		TopP:          req.TopP,
		StopSequences: req.StopSequences,
	}
	b, _ := json.Marshal(fields)
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h)
}
