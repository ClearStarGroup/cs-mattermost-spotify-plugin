import * as React from 'react';
import {connect} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';

import {getUserStatus} from './index';

type Props = {
    state: GlobalState;
};

type UserStatusCache = {
    [userId: string]: {
        isPlaying: boolean;
        lastChecked: number;
    };
};

type UsernameToIdCache = {
    [username: string]: string;
};

class UserMusicIndicator extends React.PureComponent<Props, {cache: UserStatusCache; usernameCache: UsernameToIdCache}> {
    private observer: MutationObserver | null = null;
    private checkInterval: NodeJS.Timeout | null = null;

    constructor(props: Props) {
        super(props);
        this.state = {
            cache: {},
            usernameCache: {},
        };
    }

    componentDidMount() {
        // Watch for DOM changes to detect new usernames
        this.observer = new MutationObserver(() => {
            this.updateMusicIcons();
        });

        // Start observing the document body for changes
        this.observer.observe(document.body, {
            childList: true,
            subtree: true,
        });

        // Initial update
        this.updateMusicIcons();

        // Periodically refresh status (every 15 seconds)
        this.checkInterval = setInterval(() => {
            this.updateMusicIcons();
        }, 15000);
    }

    componentWillUnmount() {
        if (this.observer) {
            this.observer.disconnect();
        }
        if (this.checkInterval) {
            clearInterval(this.checkInterval);
        }
    }

    getUserIdFromUsername = async (username: string): Promise<string | null> => {
        // Check cache first
        if (this.state.usernameCache[username]) {
            return this.state.usernameCache[username];
        }

        try {
            // Fetch user data from Mattermost API
            const response = await fetch(`/api/v4/users/username/${username}`);
            if (!response.ok) {
                return null;
            }
            const userData = await response.json();
            const userId = userData.id;

            // Cache the username to ID mapping
            this.setState((prevState) => ({
                usernameCache: {
                    ...prevState.usernameCache,
                    [username]: userId,
                },
            }));

            return userId;
        } catch (error) {
            // Failed to fetch user data
            return null;
        }
    };

    updateMusicIcons = async () => {
        console.log('updateMusicIcons');

        // Find all username buttons in posts
        const usernameButtons = document.querySelectorAll('button.user-popover');
        const userMap = new Map<string, HTMLElement[]>(); // userId -> elements

        // Extract usernames and look up user IDs
        for (const button of Array.from(usernameButtons)) {
            const username = button.textContent?.trim().replace(/^@/, '');
            if (!username) {
                continue;
            }
            console.log('username', username);

            // eslint-disable-next-line no-await-in-loop
            const userId = await this.getUserIdFromUsername(username);
            if (userId) {
                if (!userMap.has(userId)) {
                    userMap.set(userId, []);
                }
                userMap.get(userId)?.push(button as HTMLElement);
            }
            console.log('userId', userId);
        }

        // Check status for each user (with caching)
        const now = Date.now();
        const CACHE_DURATION = 30000; // 30 seconds

        for (const [userId, elements] of userMap.entries()) {
            const cached = this.state.cache[userId];

            // Skip if recently checked
            if (cached && (now - cached.lastChecked) < CACHE_DURATION) {
                elements.forEach((element) => this.addIconToElement(element, cached.isPlaying));
                continue;
            }

            try {
                // eslint-disable-next-line no-await-in-loop
                const status = await getUserStatus(this.props.state, userId);
                const isPlaying = status && (status as any).IsPlaying;

                // Update cache
                this.setState((prevState) => ({
                    cache: {
                        ...prevState.cache,
                        [userId]: {
                            isPlaying,
                            lastChecked: now,
                        },
                    },
                }));

                elements.forEach((element) => this.addIconToElement(element, isPlaying));
            } catch (error) {
                // User doesn't have Spotify connected or status not available
                // Update cache to avoid repeated failed requests
                this.setState((prevState) => ({
                    cache: {
                        ...prevState.cache,
                        [userId]: {
                            isPlaying: false,
                            lastChecked: now,
                        },
                    },
                }));
                elements.forEach((element) => this.addIconToElement(element, false));
            }
        }
    };

    addIconToElement = (element: HTMLElement, isPlaying: boolean) => {
        console.log('addIconToElement', element, isPlaying);

        // Check if we've already added an indicator to this element
        const existingIndicator = element.parentElement?.querySelector(':scope > .spotify-music-indicator');

        if (isPlaying && !existingIndicator) {
            // Add music indicator
            const indicator = document.createElement('span');
            indicator.className = 'spotify-music-indicator';
            indicator.innerHTML = ' ♫';
            indicator.style.cssText = 'color: #1DB954; font-size: 14px; font-weight: bold;';
            indicator.title = 'Listening to Spotify';

            element.parentNode?.insertBefore(indicator, element.nextSibling);
        } else if (!isPlaying && existingIndicator) {
            // Remove indicator if user stopped playing
            existingIndicator.remove();
        }
    };

    render() {
        // This component doesn't render anything visible
        // It just manages the music indicators in the DOM
        return null;
    }
}

const mapStateToProps = (state: GlobalState) => ({
    state,
});

export default connect(mapStateToProps)(UserMusicIndicator);

