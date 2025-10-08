// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

import type {Store, Action} from 'redux';

import type {GlobalState} from '@mattermost/types/store';

import {getConfig} from 'mattermost-redux/selectors/entities/general';

import manifest from './manifest';
import StatusComponent from './StatusComponent';
import type {PluginRegistry} from './types/mattermost-webapp';
import UserMusicIndicator from './UserMusicIndicator';

export type PlayerStatus = {
    IsConnected: boolean;
    IsPlaying: boolean;
    PlaybackType: string;
    PlaybackURL: string;
    PlaybackName: string;
};

export const getPluginServerRoute = (state: GlobalState) => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
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

export function getUserStatus(state: GlobalState, userId: string): Promise<PlayerStatus> {
    return new Promise((resolve, reject) => fetch(getPluginServerRoute(state) + '/api/v1/status/' + userId).then((r) => r.json()).then(resolve).catch(reject));
}

export default class Plugin {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    public async initialize(registry: PluginRegistry, store: Store<GlobalState, Action<Record<string, unknown>>>) {
        // Register the component that shows Spotify status on user profiles
        registry.registerPopoverUserAttributesComponent(StatusComponent);

        // Register the component that shows music icons next to usernames
        registry.registerGlobalComponent(UserMusicIndicator);
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
