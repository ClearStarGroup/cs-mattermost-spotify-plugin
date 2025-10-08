// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import type {Store, Action} from 'redux';

import type {GlobalState} from '@mattermost/types/store';

import {getConfig} from 'mattermost-redux/selectors/entities/general';

import manifest from './manifest';
import StatusComponent from './StatusComponent';
import type {PluginRegistry} from './types/mattermost-webapp';
import UserMusicIndicator from './UserMusicIndicator';

export const getPluginServerRoute = (state: GlobalState) => {
    const config = getConfig(state as any);

    let basePath = '/';

    if (config && config.SiteURL) {
        basePath = new URL(config.SiteURL).pathname;

        if (basePath && basePath[basePath.length - 1] === '/') {
            basePath = basePath.substr(0, basePath.length - 1);
        }
    }

    return basePath + '/plugins/' + manifest.id;
};

function getMyStatus(state: GlobalState) {
    return new Promise((resolve, reject) => fetch(getPluginServerRoute(state) + '/api/v1/me').then((r) => r.json()).then(resolve).catch(reject));
}

export function getUserStatus(state: GlobalState, userId: string) {
    return new Promise((resolve, reject) => fetch(getPluginServerRoute(state) + '/api/v1/status/' + userId).then((r) => r.json()).then(resolve).catch(reject));
}

export default class Plugin {
    public async initialize(registry: PluginRegistry, store: Store<GlobalState, Action<Record<string, unknown>>>) {
        // Register the component that shows Spotify status on user profiles
        registry.registerPopoverUserAttributesComponent(StatusComponent);

        // Register the component that shows music icons next to usernames
        registry.registerGlobalComponent(UserMusicIndicator);

        const state = store.getState();

        // Calls the server /me plugin endpoint to cache my Spotify status
        const updateState = () => {
            getMyStatus(state).then(() => {
                // Successfully updated backend cache
            }).catch(() => {
                // Silently fail if user hasn't connected Spotify
            });
        };

        // Update backend cached status every 10 seconds
        setInterval(updateState, 10 * 1000);

        // Initial status update
        updateState();
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
