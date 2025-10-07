import * as React from 'react';
import {connect} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';

import type {SpotifyPlayerState} from './model';

import {getUserStatus} from './index';

type Props = {
    state?: GlobalState;
    UID?: string;
};

class SpotifyInfo extends React.PureComponent<Props, {status: SpotifyPlayerState | null}> {
    constructor(props: Props) {
        super(props);
        this.state = {status: null};
    }

    componentDidMount() {
        // Fetch cached status for any user viewing their profile
        if (this.props.state && this.props.UID) {
            getUserStatus(this.props.state, this.props.UID).then((data) => {
                this.setState({status: data as SpotifyPlayerState});
            }).catch(() => {
                // Silently fail if user hasn't connected Spotify or status not cached
            });
        }
    }

    render() {
        if (!this.state.status) {
            return (<span>{'Spotify: Not connected'}</span>);
        }
        if (!this.state.status.is_playing) {
            return (<span>{'Spotify: Not playing'}</span>);
        }
        return (<span>{'Spotify Playing: '}{this.state.status.item.name}{' by '}{this.state.status.item.artists[0].name}</span>);
    }
}

const mapStateToProps = (state: any, ownProps: any) => {
    const UID = ownProps.user ? ownProps.user.id : '';
    return ({
        state,
        UID,
    });
};

export default connect(mapStateToProps)(SpotifyInfo);
