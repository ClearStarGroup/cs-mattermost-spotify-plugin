package kvstore

import (
	"encoding/json"

	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

// Impl implements the KVStore interface for Spotify plugin data
type Impl struct {
	pluginAPI PluginAPI
}

// NewKVStore creates a new KVStore client
func NewKVStore(pluginAPI PluginAPI) (KVStore, error) {
	return &Impl{
		pluginAPI: pluginAPI,
	}, nil
}

// StoreUserEmail stores the bidirectional mapping between user ID and Spotify email
func (kv *Impl) StoreUserEmail(userID, email string) error {
	// Store email -> userID mapping
	err := kv.pluginAPI.KVSet("email-"+email, []byte(userID))
	if err != nil {
		return errors.Wrap(err, "failed to store email mapping")
	}

	// Store userID -> email mapping
	err = kv.pluginAPI.KVSet("uid-"+userID, []byte(email))
	if err != nil {
		return errors.Wrap(err, "failed to store user ID mapping")
	}

	return nil
}

// GetUserIDByEmail retrieves the user ID associated with a Spotify email
func (kv *Impl) GetUserIDByEmail(email string) (string, error) {
	userID, err := kv.pluginAPI.KVGet("email-" + email)
	if err != nil {
		return "", errors.Wrap(err, "failed to get user ID by email")
	}
	if userID == nil {
		return "", errors.New("no user ID found for email")
	}
	return string(userID), nil
}

// GetEmailByUserID retrieves the Spotify email associated with a user ID
func (kv *Impl) GetEmailByUserID(userID string) (string, error) {
	email, err := kv.pluginAPI.KVGet("uid-" + userID)
	if err != nil {
		return "", errors.Wrap(err, "failed to get email by user ID")
	}
	if email == nil {
		return "", errors.New("no email found for user ID")
	}
	return string(email), nil
}

// StoreToken stores the OAuth token for a user
func (kv *Impl) StoreToken(userID string, token *oauth2.Token) error {
	if token == nil {
		return errors.New("cannot store nil token")
	}

	tokenJSON, err := json.Marshal(token)
	if err != nil {
		return errors.Wrap(err, "failed to marshal token")
	}

	err = kv.pluginAPI.KVSet("token-"+userID, tokenJSON)
	if err != nil {
		return errors.Wrap(err, "failed to store token")
	}

	return nil
}

// GetToken retrieves the OAuth token for a user
func (kv *Impl) GetToken(userID string) (*oauth2.Token, error) {
	kv.pluginAPI.LogInfo("Getting token for user", "userID", userID)

	tokenJSON, err := kv.pluginAPI.KVGet("token-" + userID)
	if err != nil || tokenJSON == nil {
		return nil, errors.Wrap(err, "failed to get token")
	}

	kv.pluginAPI.LogInfo("Got token for user", "tokenJSON", string(tokenJSON), "len", len(tokenJSON), "isNil", tokenJSON == nil)

	if len(tokenJSON) == 0 {
		return nil, nil
	}

	kv.pluginAPI.LogInfo("Unmarshalling token for user", "tokenJSON", string(tokenJSON), "len", len(tokenJSON), "isNil", tokenJSON == nil)

	var token oauth2.Token
	if err := json.Unmarshal(tokenJSON, &token); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal token")
	}

	kv.pluginAPI.LogInfo("Unmarshalled token for user", "token", token)

	return &token, nil
}

// CacheStatus stores the Spotify player status for a user with configurable expiration
func (kv *Impl) StoreCacheStatus(userID string, status *Status) error {
	if status == nil {
		// Delete cached status if nil
		err := kv.pluginAPI.KVDelete("cached-status-" + userID)
		if err != nil {
			return errors.Wrap(err, "failed to delete cached status")
		}

		return nil
	}

	statusJSON, err := json.Marshal(status)
	if err != nil {
		return errors.Wrap(err, "failed to marshal status")
	}

	// Get cache duration from configuration
	expirationMinutes := kv.pluginAPI.GetStatusCacheDurationMinutes()
	expirationSeconds := int64(expirationMinutes * 60)

	// Set with configurable expiration using the API directly
	appErr := kv.pluginAPI.KVSet("cached-status-"+userID, statusJSON, expirationSeconds)
	if appErr != nil {
		return errors.Wrap(appErr, "failed to cache status")
	}

	return nil
}

// GetCachedStatus retrieves the cached Spotify player status for a user
func (kv *Impl) GetCachedStatus(userID string) (*Status, error) {
	statusJSON, err := kv.pluginAPI.KVGet("cached-status-" + userID)
	if err != nil || statusJSON == nil {
		return nil, errors.Wrap(err, "failed to get cached status")
	}

	if len(statusJSON) == 0 {
		return nil, nil
	}

	var status Status
	if err := json.Unmarshal(statusJSON, &status); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal status")
	}

	return &status, nil
}

// ClearUserData removes all data associated with a user (mappings, token, and cached status)
func (kv *Impl) ClearUserData(userID string) error {
	// Get the email first so we can delete both mappings
	email, err := kv.GetEmailByUserID(userID)
	if err == nil && email != "" {
		_ = kv.pluginAPI.KVDelete("email-" + email)
	}

	// Delete the user ID mapping
	_ = kv.pluginAPI.KVDelete("uid-" + userID)

	// Delete the OAuth token
	_ = kv.pluginAPI.KVDelete("token-" + userID)

	// Delete the cached status
	_ = kv.pluginAPI.KVDelete("cached-status-" + userID)

	return nil
}
