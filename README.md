# Clear Star Mattermost Spotify Plugin

A Mattermost plugin that integrates with Spotify to display users' currently playing music on their Mattermost profile. Originally forked from [reflog/mattermost-spotify](https://github.com/reflog/mattermost-spotify), this plugin has been completely rewritten on top of the [Mattermost plugin starter template](https://github.com/mattermost/mattermost-plugin-starter-template).

[![Build Status](https://github.com/mattermost/mattermost-plugin-starter-template/actions/workflows/ci.yml/badge.svg)](https://github.com/mattermost/mattermost-plugin-starter-template/actions/workflows/e2e.yml)

## Overview

This plugin enables Mattermost users to share their Spotify listening activity with their teammates. When a user has the plugin enabled and is playing music on Spotify, their current track information appears in their Mattermost profile popover and as a music icon next to their username in posts.

### Features

- Spotify OAuth authentication
- Automatic status caching (configurable TTL, defaulting to 15 mins)
- Status displayed in user profile popover
- Music icons (♫) next to usernames in posts when actively playing
- Supports all Spotify playback types (playlist, album, artist, show)

### How It Works

1. User runs `/spotify enable their@spotify.email.com` command
2. Redirected to Spotify for OAuth authorization
3. Returns to Mattermost with connected Spotify account
4. User profile cards include what they're listerning to
5. Music icon is show next to usernames if they're listening to Spotify

## Installation

### Prerequisites

- Mattermost server v7.0 or later
- Node.js v16+ and npm v8+
- Go 1.19 or later

### Building the Plugin

```bash
# Clone repository
git clone https://github.com/clearstargroup/cs-mattermost-spotify-plugin.git
cd cs-mattermost-spotify-plugin

# Build the plugin
make dist
```

This produces `dist/com.clearstargroup.cs-mattermost-spotify-plugin.tar.gz`

### Development

For development with hot-reloading:

```bash
# Set environment variables
export MM_SERVICESETTINGS_SITEURL=http://localhost:8065
export MM_ADMIN_TOKEN=your-admin-token

# Build and deploy
make deploy

# Watch for changes
make watch
```

## Configuration

### 1. Create Spotify Application

1. Go to [Spotify Developer Dashboard](https://developer.spotify.com/dashboard/applications)
2. Create a new app
3. Set redirect URI to:
   ```
   https://YOUR-MATTERMOST-URL/plugins/com.clearstargroup.cs-mattermost-spotify-plugin/callback
   ```
4. Save your **Client ID** and **Client Secret**
5. Add e-mail address of all users to the App User Management tab to enable them to authenticate against the app if App status is Development Mode

### 2. Configure Plugin

1. Upload plugin bundle in **System Console** → **Plugins** → **Plugin Management**
2. Under the plugin sessions, enter Client ID and Client Secret, and adjust cache duration if required
3. Click **Save** and **Enable**

## Usage

### User Setup

Users enable their personal integration:

```bash
/spotify enable your@spotify.email.com
/spotify disable    # To disconnect
/spotify refresh    # To clear status cache
```

Then complete Spotify authorization in the browser.

### Status Display

**Profile Popover:**
- Shows "Spotify: Not connected" if not configured
- Shows "Spotify: Not playing" when no active playback
- Shows "Spotify: Playing [Type] - [Name]" with clickable link when active

**Post Indicators:**
- Green music icon (♫) appears next to usernames when actively playing
- Updates every 15 seconds

## Code Structure

### Server (Go)

```
server/
├── plugin.go           # Core plugin lifecycle
├── api.go              # HTTP handlers for web frontend and OAuth
├── configuration.go    # Plugin configuration
├── command/
|   ├── command.go      # Interface for slash command handler
│   └── command_impl.go # Slash command handlers
└── store/kvstore/
    ├── kvstore.go      # Interface for data persistance layer
    └── kvstore_impl.go # Data persistence layer
```

**Key Components:**
- `api.go`: OAuth callback handler and `/api/v1/status/{userId}` endpoint
- `command/command_impl.go`: Implements `/spotify enable|disable|refresh` commands
- `kvstore/`: Manages user tokens, email mappings, and status caching

**API Endpoints:**
- `POST /callback` - OAuth callback (public)
- `GET /api/v1/status/{userId}` - Get cached/current Spotify status (authenticated)

### Webapp (TypeScript/React)

```
webapp/src/
├── index.tsx              # Plugin initialization
├── StatusComponent.tsx    # Profile popover component
└── UserMusicIndicator.tsx # Username indicator component
```

**Components:**
- `StatusComponent.tsx`: Shows Spotify status in user profile popover
- `UserMusicIndicator.tsx`: Adds music icons next to usernames in posts, watches DOM changes

## Technical Details

### OAuth Scopes

- `user-read-playback-state` - Read current playback state
- `user-read-email` - Associate with Mattermost user
- `user-read-private` - User profile info

### Status Caching

The plugin implements a two-level caching system to minimize Spotify API calls and improve response times:

**Status Caching:**
- User playback status cached in KV store with (configurable) 15 minutes expiration
- Cached status includes: connection state, playing state, playback type, URL, and context name
- Automatically refreshed on next request when cache expires
- Status can be manually cleared with `/spotify refresh` command

**Context Name Caching:**
- Context names (playlist, artist, album, podcast show) are cached separately to avoid repeated API calls
- Supports all Spotify playback types: Artist, Playlist, Album, Show
- When a user is listening to a playlist, artist page, album, or podcast episode, the plugin fetches and caches the relevant name
- For inaccessible playlists, falls back to web scraping to extract the playlist name
- Cache persists indefinitely to minimize API calls for frequently accessed content

**KV store structure**:
  - `token-{userId}` - OAuth token
  - `status-{userId}` - Cached playback status
  - `email-{email}` - Email to user ID mapping
  - `context-{type}-{id}` - Context name cache (playlist/artist/album/show names)

**Web Front End Caching:**
The web front end also caches users statuses for 30 seconds to avoid repeated calls to the backend if profiles are viewed multiple times or usernames occur multiple times on a page.

### Data Flow

1. User enables plugin → registers email, gets OAuth URL
2. OAuth callback → verifies email matches, stores token
3. Status fetch → checks cache, fetches from Spotify API if needed
4. Cache expires → automatically refreshed on next request
5. Webapp → reads cached status from `/api/v1/status/{userId}`