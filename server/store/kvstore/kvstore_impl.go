package kvstore

import (
	"encoding/json"

	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
)

// Impl implements the KVStore interface for Spotify plugin data
type Impl struct {
	client *pluginapi.Client
	api    plugin.API
}

// NewKVStore creates a new KVStore client
func NewKVStore(client *pluginapi.Client, api plugin.API) KVStore {
	return &Impl{
		client: client,
		api:    api,
	}
}

// StoreUserEmail stores the bidirectional mapping between user ID and Spotify email
func (kv *Impl) StoreUserEmail(userID, email string) error {
	// Store email -> userID mapping
	_, err := kv.client.KV.Set("email-"+email, []byte(userID))
	if err != nil {
		return errors.Wrap(err, "failed to store email mapping")
	}

	// Store userID -> email mapping
	_, err = kv.client.KV.Set("uid-"+userID, []byte(email))
	if err != nil {
		return errors.Wrap(err, "failed to store user ID mapping")
	}

	return nil
}

// GetUserIDByEmail retrieves the user ID associated with a Spotify email
func (kv *Impl) GetUserIDByEmail(email string) (string, error) {
	var userID []byte
	err := kv.client.KV.Get("email-"+email, &userID)
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
	var email []byte
	err := kv.client.KV.Get("uid-"+userID, &email)
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

	_, err = kv.client.KV.Set("token-"+userID, tokenJSON)
	if err != nil {
		return errors.Wrap(err, "failed to store token")
	}

	return nil
}

// GetToken retrieves the OAuth token for a user
func (kv *Impl) GetToken(userID string) (*oauth2.Token, error) {
	var tokenJSON []byte
	err := kv.client.KV.Get("token-"+userID, &tokenJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get token")
	}

	if tokenJSON == nil {
		return nil, errors.New("no token found for user")
	}

	var token oauth2.Token
	if err := json.Unmarshal(tokenJSON, &token); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal token")
	}

	return &token, nil
}

// CacheStatus stores the Spotify player status for a user with 30-second expiration
func (kv *Impl) CacheStatus(userID string, status *Status) error {
	if status == nil {
		// Delete cached status if nil
		_ = kv.client.KV.Delete("cached-status-" + userID)
		return nil
	}

	statusJSON, err := json.Marshal(status)
	if err != nil {
		return errors.Wrap(err, "failed to marshal status")
	}

	// Set with 30-second expiration using the API directly
	appErr := kv.api.KVSetWithExpiry("cached-status-"+userID, statusJSON, 30)
	if appErr != nil {
		return errors.Wrap(appErr, "failed to cache status")
	}

	return nil
}

// GetCachedStatus retrieves the cached Spotify player status for a user
func (kv *Impl) GetCachedStatus(userID string) (*Status, error) {
	var statusJSON []byte
	err := kv.client.KV.Get("cached-status-"+userID, &statusJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cached status")
	}

	if statusJSON == nil {
		return nil, errors.New("no cached status found for user")
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
		_ = kv.client.KV.Delete("email-" + email)
	}

	// Delete the user ID mapping
	_ = kv.client.KV.Delete("uid-" + userID)

	// Delete the OAuth token
	_ = kv.client.KV.Delete("token-" + userID)

	// Delete the cached status
	_ = kv.client.KV.Delete("cached-status-" + userID)

	return nil
}
