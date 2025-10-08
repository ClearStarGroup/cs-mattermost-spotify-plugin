package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/clearstargroup/cs-mattermost-spotify-plugin/server/store/kvstore"
	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/pkg/errors"
	"github.com/zmb3/spotify/v2"
)

// MatterMost plugin hook - invoked when an HTTP request is received.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	router := mux.NewRouter()

	// Public routes (no auth required)
	router.HandleFunc("/callback", p.handleSpotifyCallback).Methods(http.MethodGet)

	// Routes that require authentication
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.Use(p.MattermostAuthorizationRequired)

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
		p.API.LogError("Spotify not configured")
		http.Error(w, "Spotify not configured", http.StatusInternalServerError)
		return
	}

	if st := r.FormValue("state"); st != "123" {
		p.API.LogError("State mismatch", "state", st)
		http.NotFound(w, r)
		return
	}

	ctx := context.Background()
	tok, err := p.auth.Token(ctx, "123", r)
	if err != nil {
		p.API.LogError("Failed to get token", "error", err)
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		return
	}

	httpClient := p.auth.Client(ctx, tok)
	cli := spotify.New(httpClient)
	cu, err := cli.CurrentUser(ctx)
	if err != nil {
		p.API.LogError("Failed to get current user", "error", err)
		http.Error(w, "Couldn't get user", http.StatusForbidden)
		return
	}

	// Verify the user has previously registered with this email
	userID, err := p.kvstore.GetUserIDByEmail(cu.Email)
	if err != nil {
		p.API.LogError("No user ID found for email", "email", cu.Email, "error", err)
		http.Error(w, "no registration found for "+cu.Email, http.StatusForbidden)
		return
	}

	// Verify the email mapping is bidirectional
	email, err := p.kvstore.GetEmailByUserID(userID)
	if err != nil || email != cu.Email {
		p.API.LogError("Invalid user mapping", "userID", userID, "error", err)
		http.Error(w, "invalid user mapping", http.StatusForbidden)
		return
	}

	// Store the OAuth token
	if err := p.kvstore.StoreToken(userID, tok); err != nil {
		p.API.LogError("Failed to store token", "error", err)
		http.Error(w, "failed to store token", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Successfully connected to Spotify! You can close this window.")); err != nil {
		p.API.LogError("Failed to write response", "error", err)
		http.Error(w, "failed to write response", http.StatusInternalServerError)
		return
	}

	p.API.LogInfo("Successfully handled Spotify callback", "email", cu.Email, "userID", userID)
}

// handleStatus returns the Spotify player status for any user, fetching and caching if necessary
func (p *Plugin) handleStatus(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		p.API.LogError("Invalid requesting user", "userID", userID)
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	// Try to get cached status first
	status, err := p.kvstore.GetCachedStatus(userID)
	if err != nil {
		p.API.LogError("Failed to get cached status", "error", err)
		http.Error(w, "failed to get cached status", http.StatusInternalServerError)
		return
	}

	// If no cached status, fetch fresh status from Spotify
	if status == nil {
		status, err = p.fetchStatus(userID)
		if err != nil {
			p.API.LogError("Failed to fetch status", "error", err)
			http.Error(w, "failed to fetch status", http.StatusInternalServerError)
			return
		}

		// Cache status
		err = p.kvstore.StoreCacheStatus(userID, status)
		if err != nil {
			p.API.LogError("Failed to cache status", "error", err)
			http.Error(w, "failed to cache status", http.StatusInternalServerError)
			return
		}
	}

	// Return status
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		p.API.LogError("Failed to encode response", "error", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	p.API.LogInfo("Successfully returned status", "userID", userID, "status", status)
}

// fetches the Spotify status for a user
func (p *Plugin) fetchStatus(userID string) (*kvstore.Status, error) {
	if p.auth == nil {
		return nil, errors.New("Spotify not configured")
	}

	ctx := context.Background()

	// Get token from KV store for the target user
	tok, err := p.kvstore.GetToken(userID)
	if err != nil {
		return nil, errors.Wrap(err, "error reading token for user")
	}

	// If no token, return not playing
	if tok == nil {
		return &kvstore.Status{IsConnected: false}, nil
	}

	// Refresh token if it's expiring soon (within 5m30s)
	if m, _ := time.ParseDuration("5m30s"); time.Until(tok.Expiry) < m {
		newToken, tokenErr := p.auth.RefreshToken(ctx, tok)
		if tokenErr != nil || newToken == nil {
			return nil, errors.Wrap(tokenErr, "failed to refresh token")
		}

		// Store refreshed token
		err = p.kvstore.StoreToken(userID, newToken)
		if err != nil {
			return nil, errors.Wrap(err, "failed to store refreshed token")
		}
	}

	httpClient := p.auth.Client(ctx, tok)
	client := spotify.New(httpClient)

	// Get player state
	status, err := client.PlayerState(ctx)
	if err != nil || status == nil {
		return nil, errors.Wrap(err, "failed to get player state")
	}

	// Handle not playing state
	if !status.Playing {
		p.API.LogInfo("Successfully fetched status - no token", "userID", userID)
		return &kvstore.Status{IsConnected: false, IsPlaying: false}, nil
	}

	// Get additional context info based on what's currently playing
	var contextName string
	var ID = spotify.ID(strings.Split(string(status.PlaybackContext.URI), ":")[2])

	// Try to get cached context name first
	contextName, err = p.kvstore.GetContextName(status.PlaybackContext.Type, string(ID))
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cached context name")
	}

	// If not in cache, fetch from Spotify API
	if contextName == "" {
		// Cache miss - fetch from Spotify API
		switch status.PlaybackContext.Type {
		case "artist":
			artist, err := client.GetArtist(ctx, ID)
			if err != nil || artist == nil {
				return nil, errors.Wrap(err, "failed to get artist")
			}
			contextName = artist.Name
		case "playlist":
			playlist, err := client.GetPlaylist(ctx, ID)
			switch {
			case err != nil && err.Error() == "Resource not found" && status.PlaybackContext.ExternalURLs["spotify"] != "":
				var resp *http.Response
				resp, err = http.Get(status.PlaybackContext.ExternalURLs["spotify"])
				if err == nil && resp != nil {
					defer resp.Body.Close()
					var body []byte
					body, err = io.ReadAll(resp.Body)
					if err == nil {
						titleStart := strings.Index(string(body), "<title>")
						titleEnd := strings.Index(string(body), "</title>")
						if titleStart >= 0 && titleEnd > titleStart {
							contextName = strings.TrimSuffix(string(body[titleStart+7:titleEnd]), " | Spotify Playlist")
						}
					}
				}
			case err != nil || playlist == nil:
				return nil, errors.Wrap(err, "failed to get playlist")
			default:
				contextName = playlist.Name
			}
		case "album":
			album, err := client.GetAlbum(ctx, ID)
			if err != nil || album == nil {
				return nil, errors.Wrap(err, "failed to get album")
			}
			contextName = album.Name + " - " + album.Artists[0].Name
		case "show":
			show, err := client.GetShow(ctx, ID)
			if err != nil || show == nil {
				return nil, errors.Wrap(err, "failed to get show")
			}
			contextName = show.Name
		}

		// Cache the fetched context name for future use
		if contextName != "" {
			if err := p.kvstore.StoreContextName(status.PlaybackContext.Type, string(ID), contextName); err != nil {
				p.API.LogError("Failed to cache context name", "type", status.PlaybackContext.Type, "id", ID, "error", err)
				// Don't return error - just log it and continue
			}
		}

		p.API.LogInfo("Successfully fetched context name", "type", status.PlaybackContext.Type, "id", ID, "name", contextName)
	}

	// Create the status result
	statusResult := &kvstore.Status{
		IsConnected:  true,
		IsPlaying:    true,
		PlaybackType: strings.ToUpper(string(status.PlaybackContext.Type[0])) + status.PlaybackContext.Type[1:],
		PlaybackURL:  status.PlaybackContext.ExternalURLs["spotify"],
		PlaybackName: contextName,
	}

	p.API.LogInfo("Successfully fetched status", "userID", userID, "status", statusResult)

	return statusResult, nil
}
