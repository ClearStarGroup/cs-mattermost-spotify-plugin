package kvstore

import (
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
)

// KVStore defines the interface for Spotify plugin key-value storage operations
type KVStore interface {
	// User authentication mappings
	StoreUserEmail(userID, email string) error
	GetUserIDByEmail(email string) (string, error)
	GetEmailByUserID(userID string) (string, error)

	// OAuth token management
	StoreToken(userID string, token *oauth2.Token) error
	GetToken(userID string) (*oauth2.Token, error)

	// Status caching
	CacheStatus(userID string, status *spotify.PlayerState) error
	GetCachedStatus(userID string) (*spotify.PlayerState, error)

	// User data cleanup
	ClearUserData(userID string) error
}
