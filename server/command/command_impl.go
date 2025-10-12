package command

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

// Impl implements the Command interface
type Impl struct {
	pluginAPI PluginAPI
}

const spotifyCommandTrigger = "spotify"

// NewCommand creates a new Command handler and registers slash commands
func NewCommand(pluginAPI PluginAPI) (Command, error) {
	// Autocomplete data
	autocompleteData := model.NewAutocompleteData("spotify", "", "Enables or disables Spotify integration.")
	autocompleteData.AddStaticListArgument("", true, []model.AutocompleteListItem{
		{
			Item:     "enable",
			HelpText: "Enable Spotify integration",
		},
		{
			Item:     "disable",
			HelpText: "Disable Spotify integration",
		},
	})

	// Register command
	err := pluginAPI.RegisterCommand(&model.Command{
		Trigger:          spotifyCommandTrigger,
		AutoComplete:     true,
		AutoCompleteDesc: "Spotify integration",
		AutoCompleteHint: "(enable|disable)",
		AutocompleteData: autocompleteData,
	})

	return &Impl{
		pluginAPI: pluginAPI,
	}, err
}

// Handle executes the commands that were registered in the NewCommandHandler function
func (c *Impl) Handle(args *model.CommandArgs) (*model.CommandResponse, error) {
	trigger := strings.TrimPrefix(strings.Fields(args.Command)[0], "/")
	switch trigger {
	case spotifyCommandTrigger:
		return c.executeSpotifyCommand(args)
	default:
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Unknown command: %s", args.Command),
		}, nil
	}
}

func (c *Impl) executeSpotifyCommand(args *model.CommandArgs) (*model.CommandResponse, error) {
	parts := strings.Fields(args.Command)
	if len(parts) < 2 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Only enable/disable commands are supported!\nUsage:\n  /spotify enable your@spotifyemail.com\n  /spotify disable",
		}, nil
	}

	switch parts[1] {
	case "enable":
		if len(parts) != 3 {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "Syntax: /spotify enable your@spotifyemail.com",
			}, nil
		}
		email := parts[2]

		if err := c.pluginAPI.StoreUserEmail(args.UserId, email); err != nil {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "Failed to store email: " + err.Error(),
			}, nil
		}

		url, err := c.pluginAPI.GetSpotifyAuthURL()
		if err != nil {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "Failed to generate auth URL: " + err.Error(),
			}, nil
		}

		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			GotoLocation: url,
			Text:         "Complete the authorization process in the new window to authorize with Spotify!",
		}, nil

	case "disable":
		if err := c.pluginAPI.ClearUserData(args.UserId); err != nil {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "Failed to disable: " + err.Error(),
			}, nil
		}
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Disabled Spotify integration!",
		}, nil

	default:
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Only enable/disable commands are supported!\nUsage:\n  /spotify enable your@spotifyemail.com\n  /spotify disable",
		}, nil
	}
}
