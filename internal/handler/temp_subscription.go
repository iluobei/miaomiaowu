package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// reorderProxyProperties reorders proxy map properties with name, type, server, port first
// Returns a yaml.Node to preserve key order
func reorderProxyToYAMLNode(proxy map[string]any) *yaml.Node {
	priorityKeys := []string{"name", "type", "server", "port"}
	node := &yaml.Node{
		Kind: yaml.MappingNode,
	}

	// First add priority keys in order
	for _, key := range priorityKeys {
		if val, exists := proxy[key]; exists {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
			valNode := &yaml.Node{}
			valNode.Encode(val)
			node.Content = append(node.Content, keyNode, valNode)
		}
	}

	// Then add remaining keys
	for key, val := range proxy {
		if key != "name" && key != "type" && key != "server" && key != "port" {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
			valNode := &yaml.Node{}
			valNode.Encode(val)
			node.Content = append(node.Content, keyNode, valNode)
		}
	}

	return node
}

// TempSubscription represents a temporary subscription
type TempSubscription struct {
	ID           string    `json:"id"`
	Proxies      []any     `json:"proxies"`
	MaxAccess    int       `json:"max_access"`
	AccessCount  int       `json:"access_count"`
	ExpireAt     time.Time `json:"expire_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// TempSubscriptionStore manages temporary subscriptions in memory
type TempSubscriptionStore struct {
	mu            sync.RWMutex
	subscriptions map[string]*TempSubscription
}

// Global store for temporary subscriptions
var tempSubStore = &TempSubscriptionStore{
	subscriptions: make(map[string]*TempSubscription),
}

// generateShortCode generates a random 8-character hex code
func generateTempSubCode() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Create creates a new temporary subscription
func (s *TempSubscriptionStore) Create(proxies []any, maxAccess int, expireSeconds int) *TempSubscription {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean up expired subscriptions
	s.cleanupLocked()

	id := generateTempSubCode()
	// Ensure unique ID
	for s.subscriptions[id] != nil {
		id = generateTempSubCode()
	}

	sub := &TempSubscription{
		ID:          id,
		Proxies:     proxies,
		MaxAccess:   maxAccess,
		AccessCount: 0,
		ExpireAt:    time.Now().Add(time.Duration(expireSeconds) * time.Second),
		CreatedAt:   time.Now(),
	}

	s.subscriptions[id] = sub
	return sub
}

// Get retrieves a temporary subscription by ID and increments access count
func (s *TempSubscriptionStore) Get(id string) (*TempSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub, exists := s.subscriptions[id]
	if !exists {
		return nil, errors.New("subscription not found")
	}

	// Check if expired
	if time.Now().After(sub.ExpireAt) {
		delete(s.subscriptions, id)
		return nil, errors.New("subscription expired")
	}

	// Check if max access reached
	if sub.AccessCount >= sub.MaxAccess {
		delete(s.subscriptions, id)
		return nil, errors.New("subscription access limit reached")
	}

	// Increment access count
	sub.AccessCount++

	// If this was the last allowed access, delete it
	if sub.AccessCount >= sub.MaxAccess {
		delete(s.subscriptions, id)
	}

	return sub, nil
}

// cleanupLocked removes expired subscriptions (must be called with lock held)
func (s *TempSubscriptionStore) cleanupLocked() {
	now := time.Now()
	for id, sub := range s.subscriptions {
		if now.After(sub.ExpireAt) || sub.AccessCount >= sub.MaxAccess {
			delete(s.subscriptions, id)
		}
	}
}

// TempSubscriptionHandler handles temporary subscription requests
type TempSubscriptionHandler struct{}

// NewTempSubscriptionHandler creates a new handler for temporary subscriptions
func NewTempSubscriptionHandler() http.Handler {
	return &TempSubscriptionHandler{}
}

func (h *TempSubscriptionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handleCreate(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

// CreateTempSubRequest represents the request to create a temporary subscription
type CreateTempSubRequest struct {
	Proxies       []any `json:"proxies"`
	MaxAccess     int   `json:"max_access"`
	ExpireSeconds int   `json:"expire_seconds"`
}

// CreateTempSubResponse represents the response after creating a temporary subscription
type CreateTempSubResponse struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	MaxAccess int       `json:"max_access"`
	ExpireAt  time.Time `json:"expire_at"`
}

func (h *TempSubscriptionHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateTempSubRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}

	if len(req.Proxies) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("proxies cannot be empty"))
		return
	}

	// Set defaults
	if req.MaxAccess <= 0 {
		req.MaxAccess = 1
	}
	if req.ExpireSeconds <= 0 {
		req.ExpireSeconds = 60
	}

	// Limit max values for security
	if req.MaxAccess > 100 {
		req.MaxAccess = 100
	}
	if req.ExpireSeconds > 3600 {
		req.ExpireSeconds = 3600 // Max 1 hour
	}

	sub := tempSubStore.Create(req.Proxies, req.MaxAccess, req.ExpireSeconds)

	resp := CreateTempSubResponse{
		ID:        sub.ID,
		URL:       "/t/" + sub.ID,
		MaxAccess: sub.MaxAccess,
		ExpireAt:  sub.ExpireAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// TempSubscriptionAccessHandler handles access to temporary subscriptions
type TempSubscriptionAccessHandler struct{}

// NewTempSubscriptionAccessHandler creates a handler for accessing temporary subscriptions
func NewTempSubscriptionAccessHandler() http.Handler {
	return &TempSubscriptionAccessHandler{}
}

func (h *TempSubscriptionAccessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	// Validate User-Agent: must contain ClashMetaForAndroid or Mihomo (case-insensitive)
	userAgent := strings.ToLower(r.Header.Get("User-Agent"))
	if !strings.Contains(userAgent, "clashmetaforandroid") && !strings.Contains(userAgent, "mihomo") {
		http.Error(w, "Invalid client", http.StatusForbidden)
		return
	}

	// Extract ID from URL path: /t/{id}
	path := strings.TrimPrefix(r.URL.Path, "/t/")
	id := strings.TrimSuffix(path, "/")

	if id == "" || len(id) != 8 {
		http.NotFound(w, r)
		return
	}

	sub, err := tempSubStore.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Build YAML with ordered proxy properties using yaml.Node
	rootNode := &yaml.Node{
		Kind: yaml.MappingNode,
	}

	// Add "proxies" key
	proxiesKeyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "proxies"}
	proxiesListNode := &yaml.Node{Kind: yaml.SequenceNode}

	for _, proxy := range sub.Proxies {
		if proxyMap, ok := proxy.(map[string]any); ok {
			proxiesListNode.Content = append(proxiesListNode.Content, reorderProxyToYAMLNode(proxyMap))
		}
	}

	rootNode.Content = append(rootNode.Content, proxiesKeyNode, proxiesListNode)

	yamlData, err := MarshalYAMLWithIndent(rootNode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("failed to generate subscription"))
		return
	}

	// Fix emoji escape sequences in YAML output
	result := RemoveUnicodeEscapeQuotes(string(yamlData))

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(result))
}
