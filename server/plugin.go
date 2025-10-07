package main

import (
	"net/http"
	"sync"

	"github.com/clearstargroup/cs-mattermost-spotify-plugin/server/command"
	"github.com/clearstargroup/cs-mattermost-spotify-plugin/server/store/kvstore"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/pkg/errors"
	"github.com/zmb3/spotify"
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
	auth *spotify.Authenticator

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

	// Create instance of plugin KVStore
	p.kvstore = kvstore.NewKVStore(p.client, p.API)

	// Create instance of plugin command client
	p.command = command.NewCommand(p.client, p)

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

// Command Plugin API - generates the Spotify OAuth authorization URL
func (p *Plugin) GetSpotifyAuthURL() (string, error) {
	if p.auth == nil {
		return "", errors.New("Spotify not configured")
	}
	url := p.auth.AuthURLWithOpts("123")
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

// See https://developers.mattermost.com/extend/plugins/server/reference/
