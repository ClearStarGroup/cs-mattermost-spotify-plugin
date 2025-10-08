import * as React from 'react';
import {connect} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';

import {getUserStatus} from './index';

type Props = {
    state?: GlobalState;
    UID?: string;
};

type PlayerStatus = {
    IsPlaying: boolean;
    PlaybackType: string;
    PlaybackURL: string;
    PlaybackName: string;
};

class SpotifyInfo extends React.PureComponent<Props, {status: PlayerStatus | null}> {
    constructor(props: Props) {
        super(props);
        this.state = {status: null};
    }

    componentDidMount() {
        // Fetch cached status for any user viewing their profile
        if (this.props.state && this.props.UID) {
            getUserStatus(this.props.state, this.props.UID).then((data) => {
                this.setState({status: data as PlayerStatus});
            }).catch(() => {
                // Silently fail if user hasn't connected Spotify or status not cached
            });
        }
    }

    render() {
        if (!this.state.status) {
            return (<span>{'Spotify: Not connected'}</span>);
        }
        if (!this.state.status.IsPlaying) {
            return (<span>{'Spotify: Not playing'}</span>);
        }
        return (<>
            <span>{'Spotify: Playing '}{this.state.status.PlaybackType}</span>
            <br/>
            {/* eslint-disable-next-line @mattermost/use-external-link, react/jsx-max-props-per-line */}
            <span><a href={this.state.status.PlaybackURL} target='_blank' rel='noopener noreferrer'>{this.state.status.PlaybackName}</a></span>
        </>);
    }
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const mapStateToProps = (state: any, ownProps: any) => {
    const UID = ownProps.user ? ownProps.user.id : '';
    return ({
        state,
        UID,
    });
};

export default connect(mapStateToProps)(SpotifyInfo);
