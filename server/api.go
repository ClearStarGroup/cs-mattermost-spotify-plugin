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

	ctx := context.Background()
	tok, err := p.auth.Token(ctx, "123", r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		p.API.LogError("Failed to get token", "error", err)
		return
	}

	httpClient := p.auth.Client(ctx, tok)
	cli := spotify.New(httpClient)
	cu, err := cli.CurrentUser(ctx)
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
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		p.API.LogError("Invalid user", "userID", userID)
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	// Fetch using user's own token
	p.handleSpotify(w, r, userID, func(ctx context.Context, client *spotify.Client) (interface{}, error) {
		status, err := client.PlayerState(ctx)
		if status == nil || err != nil {
			p.API.LogError("Failed to get status", "error", err)
			if cacheErr := p.kvstore.CacheStatus(userID, nil); cacheErr != nil {
				p.API.LogError("Failed to cache nil status", "error", cacheErr)
				return nil, cacheErr
			}
			return nil, err
		}

		if !status.Playing {
			if cacheErr := p.kvstore.CacheStatus(userID, &kvstore.Status{IsPlaying: false}); cacheErr != nil {
				p.API.LogError("Failed to cache nil status", "error", cacheErr)
				return nil, cacheErr
			}
			return nil, nil
		}

		// Get additional context info based on what's currently playing
		var contextName string
		var ID = spotify.ID(strings.Split(string(status.PlaybackContext.URI), ":")[2])

		switch status.PlaybackContext.Type {
		case "artist":
			var artist *spotify.FullArtist
			artist, err = client.GetArtist(ctx, ID)
			if err == nil && artist != nil {
				contextName = artist.Name
			}
		case "playlist":
			var playlist *spotify.FullPlaylist
			playlist, err = client.GetPlaylist(ctx, ID)
			if err == nil && playlist != nil {
				contextName = playlist.Name
			}
			if err != nil && err.Error() == "Resource not found" && status.PlaybackContext.ExternalURLs["spotify"] != "" {
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
			}
		case "album":
			var album *spotify.FullAlbum
			album, err = client.GetAlbum(ctx, ID)
			if err == nil && album != nil {
				contextName = album.Name + " - " + album.Artists[0].Name
			}
		case "show":
			var show *spotify.FullShow
			show, err = client.GetShow(ctx, ID)
			if err == nil && show != nil {
				contextName = show.Name
			}
		}

		if contextName == "" || err != nil {
			p.API.LogError("Failed to get context info", "error", err)
			if cacheErr := p.kvstore.CacheStatus(userID, nil); cacheErr != nil {
				p.API.LogError("Failed to cache nil status", "error", cacheErr)
			}
			return nil, err
		}

		// Calculate the status string
		result := kvstore.Status{
			IsPlaying:    true,
			PlaybackType: strings.ToUpper(string(status.PlaybackContext.Type[0])) + status.PlaybackContext.Type[1:],
			PlaybackURL:  status.PlaybackContext.ExternalURLs["spotify"],
			PlaybackName: contextName,
		}

		// Cache the status for others to view (fire and forget)
		if cacheErr := p.kvstore.CacheStatus(userID, &result); cacheErr != nil {
			p.API.LogError("Failed to cache status", "error", cacheErr)
			return nil, cacheErr
		}

		return nil, nil
	})
}

// handleStatus returns the cached Spotify player status for any user
func (p *Plugin) handleStatus(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		p.API.LogError("Invalid user", "userID", userID)
		http.Error(w, "invalid user", http.StatusBadRequest)
		return
	}

	// Read from cache
	status, err := p.kvstore.GetCachedStatus(userID)
	if err != nil {
		p.API.LogError("No status available", "error", err)
		http.Error(w, "no status available", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		p.API.LogError("Failed to encode status response", "error", err)
	}
}

// handleSpotify is a helper function that handles Spotify API calls with token management
func (p *Plugin) handleSpotify(w http.ResponseWriter, _ *http.Request, userID string, clientCode func(ctx context.Context, client *spotify.Client) (interface{}, error)) {
	if p.auth == nil {
		http.Error(w, "Spotify not configured", http.StatusInternalServerError)
		return
	}

	ctx := context.Background()

	// Get token from KV store
	tok, err := p.kvstore.GetToken(userID)
	if err != nil {
		http.Error(w, "no token for user", http.StatusBadRequest)
		return
	}

	// Refresh token if it's expiring soon (within 5m30s)
	if m, _ := time.ParseDuration("5m30s"); time.Until(tok.Expiry) < m {
		newToken, tokenErr := p.auth.RefreshToken(ctx, tok)
		if tokenErr == nil && newToken != nil {
			tok = newToken
			// Store refreshed token
			if err2 := p.kvstore.StoreToken(userID, newToken); err2 != nil {
				p.API.LogError("Failed to store refreshed token", "error", err2)
			}
		}
	}

	httpClient := p.auth.Client(ctx, tok)
	client := spotify.New(httpClient)

	ps, err := clientCode(ctx, client)
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
