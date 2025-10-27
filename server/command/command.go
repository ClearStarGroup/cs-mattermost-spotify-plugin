package command

import (
	"github.com/mattermost/mattermost/server/public/model"
)

// PluginAPI defines the interface for accessing plugin-specific functionality
type PluginAPI interface {
	RegisterCommand(command *model.Command) error
	GetSpotifyAuthURL() (string, error)
	StoreUserEmail(userID, email string) error
	ClearUserData(userID string) error
	ClearStatusCache(userID string) error
	LogInfo(message string, args ...any)
}

// Command defines the interface for handling slash commands
type Command interface {
	Handle(args *model.CommandArgs) (*model.CommandResponse, error)
}
