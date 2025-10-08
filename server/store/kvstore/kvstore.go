package kvstore

import (
	"golang.org/x/oauth2"
)

type Status struct {
	IsConnected  bool
	IsPlaying    bool
	PlaybackType string
	PlaybackURL  string
	PlaybackName string
}

type PluginAPI interface {
	KVSet(key string, value []byte, expirationSeconds ...int64) error
	KVGet(key string) ([]byte, error)
	KVDelete(key string) error
	GetStatusCacheDurationMinutes() int
	LogInfo(message string, args ...any)
}

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
	StoreCacheStatus(userID string, status *Status) error
	GetCachedStatus(userID string) (*Status, error)

	// User data cleanup
	ClearUserData(userID string) error
}
