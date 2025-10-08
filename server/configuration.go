package main

import (
	"reflect"

	"github.com/pkg/errors"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
)

// Configuration captures the plugin's external Configuration as exposed in the Mattermost server
// Configuration, as well as values computed from the Configuration. Any public fields will be
// deserialized from the Mattermost server Configuration in OnConfigurationChange.
//
// As plugins are inherently concurrent (hooks being called asynchronously), and the plugin
// Configuration can change at any time, access to the Configuration must be synchronized. The
// strategy used in this plugin is to guard a pointer to the Configuration, and clone the entire
// struct whenever it changes. You may replace this with whatever strategy you choose.
//
// If you add non-reference types to your Configuration struct, be sure to rewrite Clone as a deep
// copy appropriate for your types.
type Configuration struct {
	ClientID     string
	ClientSecret string
}

// Clone shallow copies the configuration. Your implementation may require a deep copy if
// your configuration has reference types.
func (c *Configuration) Clone() *Configuration {
	var clone = *c
	return &clone
}

// setConfiguration replaces the active configuration under lock.
//
// Do not call setConfiguration while holding the configurationLock, as sync.Mutex is not
// reentrant. In particular, avoid using the plugin API entirely, as this may in turn trigger a
// hook back into the plugin. If that hook attempts to acquire this lock, a deadlock may occur.
//
// This method panics if setConfiguration is called with the existing configuration. This almost
// certainly means that the configuration was modified without being cloned and may result in
// an unsafe access.
func (p *Plugin) setConfiguration(configuration *Configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	if configuration != nil && p.configuration == configuration {
		// Ignore assignment if the configuration struct is empty. Go will optimize the
		// allocation for same to point at the same memory address, breaking the check
		// above.
		if reflect.ValueOf(*configuration).NumField() == 0 {
			return
		}

		panic("setConfiguration called with the existing configuration")
	}

	// Initialize Spotify authenticator if configuration is provided
	if configuration != nil && configuration.ClientID != "" && configuration.ClientSecret != "" {
		siteURL := *p.API.GetConfig().ServiceSettings.SiteURL
		callbackURL := siteURL + "/plugins/com.clearstargroup.cs-mattermost-spotify-plugin/callback"
		p.auth = spotifyauth.New(
			spotifyauth.WithRedirectURL(callbackURL),
			spotifyauth.WithScopes(
				spotifyauth.ScopeUserReadPrivate,
				spotifyauth.ScopeUserReadEmail,
				spotifyauth.ScopeUserReadPlaybackState,
			),
			spotifyauth.WithClientID(configuration.ClientID),
			spotifyauth.WithClientSecret(configuration.ClientSecret),
		)
	}

	p.configuration = configuration
}

// MatterMost plugin hook - invoked when configuration changes may have been made or on plugin activation
func (p *Plugin) OnConfigurationChange() error {
	var configuration = new(Configuration)

	// Load the public configuration fields from the Mattermost server configuration.
	if err := p.API.LoadPluginConfiguration(configuration); err != nil {
		return errors.Wrap(err, "failed to load plugin configuration")
	}

	p.setConfiguration(configuration)

	return nil
}
