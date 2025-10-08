package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/clearstargroup/cs-mattermost-spotify-plugin/server/command"
	"github.com/clearstargroup/cs-mattermost-spotify-plugin/server/store/kvstore"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// client is the Mattermost server API client.
	client *pluginapi.Client

	// kvstore is the client used to read/write KV records for this plugin.
	kvstore kvstore.KVStore

	// command is the client used to register and execute slash commands.
	command command.Command

	// auth is the Spotify authenticator (initialized in setConfiguration after configuration is loaded)
	auth *spotifyauth.Authenticator

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *Configuration
}

// MatterMost plugin hook - invoked when the plugin is activated. If an error is returned, the plugin will be deactivated.
func (p *Plugin) OnActivate() error {
	// Create standard plugin client
	p.client = pluginapi.NewClient(p.API, p.Driver)

	// Create instance of plugin KVStore with cache duration getter
	kvstore, err := kvstore.NewKVStore(p)
	if err != nil {
		return errors.Wrap(err, "failed to create KVStore")
	}
	p.kvstore = kvstore

	// Create instance of plugin command client
	command, err := command.NewCommand(p)
	if err != nil {
		return errors.Wrap(err, "failed to create Command")
	}
	p.command = command

	return nil
}

// MatterMost plugin hook - invoked when the plugin is deactivated.
func (p *Plugin) OnDeactivate() error {
	return nil
}

// MatterMost plugin hook - invoked when a command is executed.
func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	response, err := p.command.Handle(args)
	if err != nil {
		return nil, model.NewAppError("ExecuteCommand", "plugin.command.execute_command.app_error", nil, err.Error(), http.StatusInternalServerError)
	}
	return response, nil
}

// Command Plugin API - registers a command
func (p *Plugin) RegisterCommand(command *model.Command) error {
	return p.client.SlashCommand.Register(command)
}

// Command Plugin API - generates the Spotify OAuth authorization URL
func (p *Plugin) GetSpotifyAuthURL() (string, error) {
	if p.auth == nil {
		return "", errors.New("Spotify not configured")
	}
	url := p.auth.AuthURL("123")
	return url, nil
}

// Command Plugin API - stores the mapping between user ID and their Spotify email
func (p *Plugin) StoreUserEmail(userID, email string) error {
	return p.kvstore.StoreUserEmail(userID, email)
}

// Command Plugin API - removes the user's Spotify integration
func (p *Plugin) ClearUserData(userID string) error {
	// Delete all user data
	return p.kvstore.ClearUserData(userID)
}

// KVStore Plugin API - stores a value with optional expiration
func (p *Plugin) KVSet(key string, value []byte, expirationSeconds ...int64) error {
	if len(expirationSeconds) > 0 {
		res, err := p.client.KV.Set(key, value, pluginapi.SetExpiry(time.Duration(expirationSeconds[0])*time.Second))
		if err != nil {
			return errors.Wrap(err, "failed to set value with expiration")
		}
		if !res {
			return errors.New("failed to set value with expiration")
		}
		return nil
	}

	res, err := p.client.KV.Set(key, value)
	if err != nil {
		return errors.Wrap(err, "failed to set value")
	}
	if !res {
		return errors.New("failed to set value")
	}
	return nil
}

// KVStore Plugin API - gets a value
func (p *Plugin) KVGet(key string) ([]byte, error) {
	obj := []byte{}
	err := p.client.KV.Get(key, &obj)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get value")
	}
	return obj, nil
}

// KVStore Plugin API - deletes a value
func (p *Plugin) KVDelete(key string) error {
	err := p.client.KV.Delete(key)
	if err != nil {
		return errors.Wrap(err, "failed to delete value")
	}
	return nil
}

// KVStore and Command Plugin API - logging methods
func (p *Plugin) LogInfo(message string, args ...any) {
	p.API.LogInfo(message, args...)
}

func (p *Plugin) LogError(message string, args ...any) {
	p.API.LogError(message, args...)
}

// See https://developers.mattermost.com/extend/plugins/server/reference/
