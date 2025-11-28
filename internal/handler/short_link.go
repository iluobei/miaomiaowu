package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type shortLinkHandler struct {
	repo               *storage.TrafficRepository
	subscriptionHandler *SubscriptionHandler
}

// NewShortLinkHandler creates a handler for short link redirection.
func NewShortLinkHandler(repo *storage.TrafficRepository, subscriptionHandler *SubscriptionHandler) http.Handler {
	if repo == nil {
		panic("short link handler requires repository")
	}
	if subscriptionHandler == nil {
		panic("short link handler requires subscription handler")
	}

	return &shortLinkHandler{
		repo:               repo,
		subscriptionHandler: subscriptionHandler,
	}
}

func (h *shortLinkHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	// Extract composite shortcode from URL path (6 characters: 3 for file + 3 for user)
	compositeCode := strings.Trim(r.URL.Path, "/")
	if compositeCode == "" || len(compositeCode) != 6 {
		http.NotFound(w, r)
		return
	}

	// Split into file short code (first 3 chars) and user short code (last 3 chars)
	fileShortCode := compositeCode[:3]
	userShortCode := compositeCode[3:]

	// Get subscription filename by file short code
	filename, err := h.repo.GetFilenameByFileShortCode(r.Context(), fileShortCode)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			http.NotFound(w, r)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Get username by user short code
	username, err := h.repo.GetUsernameByUserShortCode(r.Context(), userShortCode)
	if err != nil {
		// User not found or invalid user short code
		http.NotFound(w, r)
		return
	}

	// TODO: User-Agent validation temporarily disabled
	// Validate User-Agent
	// userAgent := r.Header.Get("User-Agent")
	// clientType := r.URL.Query().Get("t")

	// if clientType == "" {
	// 	// Default request (no t parameter): must contain "clash"
	// 	if !isClashUA(userAgent) {
	// 		http.NotFound(w, r)
	// 		return
	// 	}
	// } else {
	// 	// With t parameter: must be mobile user-agent
	// 	if !isMobileUA(userAgent) {
	// 		http.NotFound(w, r)
	// 		return
	// 	}
	// }

	// Create a new request with authenticated context and filename parameter
	// This allows us to directly invoke the subscription handler without redirecting
	ctx := auth.ContextWithUsername(r.Context(), username)

	// Build new URL with filename parameter
	newURL := *r.URL
	q := newURL.Query()
	q.Set("filename", filename)
	// Preserve the 't' parameter if present (for client type conversion)
	if clientType := r.URL.Query().Get("t"); clientType != "" {
		q.Set("t", clientType)
	}
	newURL.RawQuery = q.Encode()

	// Create new request with updated context and URL
	newRequest := r.Clone(ctx)
	newRequest.URL = &newURL

	// Directly serve the subscription content using the subscription handler
	h.subscriptionHandler.ServeHTTP(w, newRequest)
}

// isClashUA checks if the user-agent contains "clash"
func isClashUA(ua string) bool {
	lower := strings.ToLower(ua)
	return strings.Contains(lower, "clash")
}

// isMobileUA checks if the user-agent is from a mobile device
func isMobileUA(ua string) bool {
	lower := strings.ToLower(ua)
	mobileKeywords := []string{"iphone", "ipad", "android", "mobile"}
	for _, keyword := range mobileKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// NewShortLinkResetHandler creates a handler for resetting short links.
type shortLinkResetHandler struct {
	repo *storage.TrafficRepository
}

// NewShortLinkResetHandler creates a handler for resetting user short links.
func NewShortLinkResetHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("short link reset handler requires repository")
	}

	return &shortLinkResetHandler{repo: repo}
}

func (h *shortLinkResetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get username from context (authenticated via middleware)
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
		return
	}

	h.handleReset(w, r, username)
}

func (h *shortLinkResetHandler) handleReset(w http.ResponseWriter, r *http.Request, username string) {
	// Reset short URLs for all subscriptions
	if err := h.repo.ResetAllSubscriptionShortURLs(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Return success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"message":"所有订阅的短链接已重置"}`)
}
