package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/zmb3/spotify"
)

// MatterMost plugin hook - invoked when an HTTP request is received.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	router := mux.NewRouter()

	// Public routes (no auth required)
	router.HandleFunc("/callback", p.handleSpotifyCallback).Methods(http.MethodGet)

	// Routes that require authentication
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.Use(p.MattermostAuthorizationRequired)

	apiRouter.HandleFunc("/me", p.handleMe).Methods(http.MethodGet)
	apiRouter.HandleFunc("/status/{userId}", p.handleStatus).Methods(http.MethodGet)

	router.ServeHTTP(w, r)
}

func (p *Plugin) MattermostAuthorizationRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("Mattermost-User-ID")
		if userID == "" {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleSpotifyCallback handles the OAuth callback from Spotify
func (p *Plugin) handleSpotifyCallback(w http.ResponseWriter, r *http.Request) {
	if p.auth == nil {
		http.Error(w, "Spotify not configured", http.StatusInternalServerError)
		return
	}

	if st := r.FormValue("state"); st != "123" {
		http.NotFound(w, r)
		p.API.LogError("State mismatch", "state", st)
		return
	}

	tok, err := p.auth.Token("123", r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		p.API.LogError("Failed to get token", "error", err)
		return
	}

	cli := p.auth.NewClient(tok)
	cu, err := cli.CurrentUser()
	if err != nil {
		http.Error(w, "Couldn't get user", http.StatusForbidden)
		p.API.LogError("Failed to get current user", "error", err)
		return
	}

	// Verify the user has previously registered with this email
	userID, err := p.kvstore.GetUserIDByEmail(cu.Email)
	if err != nil {
		http.Error(w, "no registration found for "+cu.Email, http.StatusForbidden)
		p.API.LogError("No user ID found for email", "email", cu.Email, "error", err)
		return
	}

	// Verify the email mapping is bidirectional
	email, err := p.kvstore.GetEmailByUserID(userID)
	if err != nil || email != cu.Email {
		http.Error(w, "invalid user mapping", http.StatusForbidden)
		p.API.LogError("Invalid user mapping", "userID", userID, "error", err)
		return
	}

	// Store the OAuth token
	if err := p.kvstore.StoreToken(userID, tok); err != nil {
		http.Error(w, "failed to store token", http.StatusInternalServerError)
		p.API.LogError("Failed to store token", "error", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Successfully connected to Spotify! You can close this window.")); err != nil {
		p.API.LogError("Failed to write response", "error", err)
	}
}

// handleMe returns the current user's Spotify player status and caches it
func (p *Plugin) handleMe(w http.ResponseWriter, r *http.Request) {
	p.API.LogInfo("handleMe", "userID", r.Header.Get("Mattermost-User-ID"))

	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		p.API.LogError("Invalid user", "userID", userID)
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	// Fetch using user's own token
	p.handleSpotify(w, r, userID, func(client *spotify.Client) (interface{}, error) {
		status, err := client.PlayerState()
		p.API.LogInfo("handleMe", "status", status, "error", err)
		if err == nil && status != nil {
			// Cache the status for others to view (fire and forget)
			go func() {
				if err2 := p.kvstore.CacheStatus(userID, status); err2 != nil {
					p.API.LogError("Failed to cache status", "error", err2)
				}
			}()
		}
		return status, err
	})
}

// handleStatus returns the cached Spotify player status for any user
func (p *Plugin) handleStatus(w http.ResponseWriter, r *http.Request) {
	p.API.LogInfo("handleStatus", "userID", r.Header.Get("Mattermost-User-ID"), "userId", mux.Vars(r)["userId"])

	vars := mux.Vars(r)
	userID := vars["userId"]

	if userID == "" {
		p.API.LogError("Invalid user", "userID", userID)
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}

	// Read from cache
	status, err := p.kvstore.GetCachedStatus(userID)
	if err != nil {
		p.API.LogError("No status available", "error", err)
		http.Error(w, "no status available", http.StatusNotFound)
		return
	}
	p.API.LogInfo("handleStatus", "status", status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		p.API.LogError("Failed to encode status response", "error", err)
	}
}

// handleSpotify is a helper function that handles Spotify API calls with token management
func (p *Plugin) handleSpotify(w http.ResponseWriter, _ *http.Request, userID string, clientCode func(client *spotify.Client) (interface{}, error)) {
	if p.auth == nil {
		http.Error(w, "Spotify not configured", http.StatusInternalServerError)
		return
	}

	// Get token from KV store
	tok, err := p.kvstore.GetToken(userID)
	if err != nil {
		http.Error(w, "no token for user", http.StatusBadRequest)
		return
	}

	client := p.auth.NewClient(tok)

	// Refresh token if it's expiring soon (within 5m30s)
	if m, _ := time.ParseDuration("5m30s"); time.Until(tok.Expiry) < m {
		newToken, tokenErr := client.Token()
		if tokenErr == nil {
			// Store refreshed token
			if err2 := p.kvstore.StoreToken(userID, newToken); err2 != nil {
				p.API.LogError("Failed to store refreshed token", "error", err2)
			}
		}
	}

	ps, err := clientCode(&client)
	if err != nil {
		http.Error(w, "cannot perform spotify commands: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(ps); err != nil {
		p.API.LogError("Failed to encode response", "error", err)
	}
}
