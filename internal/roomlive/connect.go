package roomlive

import (
	"context"

	"github.com/centrifugal/centrifuge"
)

type viewerKey struct{}

func withViewer(ctx context.Context, viewer Viewer) context.Context {
	return context.WithValue(ctx, viewerKey{}, viewer)
}

func viewerOf(ctx context.Context) (Viewer, bool) {
	viewer, ok := ctx.Value(viewerKey{}).(Viewer)
	return viewer, ok
}

// Unsubscribe codes. A code under 2500 ends the subscription for good; one at 2500 or over makes the browser subscribe again,
// which it does from the position it holds.
const (
	unsubscribeLeftRoom uint32 = 2403
)

// goingAway is how the server closes a socket when it is shutting down: 1001, which a browser takes as leave to reconnect.
var goingAway = centrifuge.Disconnect{Code: 1001, Reason: "going away"}

// conn is one open socket: whose it is and what it is allowed to do at once.
type conn struct {
	client *centrifuge.Client
	viewer Viewer
	rate   *bucket
	// slots bounds the commands carried out for this socket at the same time.
	slots chan struct{}
	typed typingGate
	// channels is what the socket is subscribed to, guarded by the server's lock.
	channels map[string]struct{}
}

func newConn(client *centrifuge.Client, viewer Viewer) *conn {
	return &conn{client: client, viewer: viewer, rate: newBucket(20, 5), slots: make(chan struct{}, 4), channels: map[string]struct{}{}}
}

// connecting accepts a connection whose request was authenticated before the upgrade. A connection with no viewer in its
// context did not come through Handler and is refused.
func (s *Server) connecting(ctx context.Context, _ centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
	if _, ok := viewerOf(ctx); !ok {
		return centrifuge.ConnectReply{}, centrifuge.DisconnectInvalidToken
	}
	return centrifuge.ConnectReply{}, nil
}

func (s *Server) connected(client *centrifuge.Client) {
	viewer, _ := viewerOf(client.Context())
	each := newConn(client, viewer)
	s.add(each)
	if s.closing.Load() {
		// It arrived after shutdown took its snapshot of the sockets.
		client.Disconnect(goingAway)
	}
	client.OnSubscribe(func(event centrifuge.SubscribeEvent, done centrifuge.SubscribeCallback) {
		s.subscribe(each, event, done)
	})
	client.OnUnsubscribe(func(event centrifuge.UnsubscribeEvent) { s.unsubscribed(each, event.Channel) })
	client.OnRPC(func(event centrifuge.RPCEvent, done centrifuge.RPCCallback) { s.command(each, event, done) })
	client.OnPresence(func(event centrifuge.PresenceEvent, done centrifuge.PresenceCallback) {
		s.presence(each, event, done)
	})
	client.OnRefresh(func(_ centrifuge.RefreshEvent, done centrifuge.RefreshCallback) { s.refresh(each, done) })
	client.OnDisconnect(func(centrifuge.DisconnectEvent) { s.remove(each) })
}

// subscribe lets a person watch a room's channel when they are in the room. The stream is positioned and recoverable, so a
// client that comes back is replayed what it missed; the live channel has presence and no position.
func (s *Server) subscribe(each *conn, event centrifuge.SubscribeEvent, done centrifuge.SubscribeCallback) {
	room, stream, ok := channelRoom(event.Channel)
	if !ok {
		done(centrifuge.SubscribeReply{}, centrifuge.ErrorUnknownChannel)
		return
	}
	if err := s.backend.Authorize(each.viewer, room); err != nil {
		done(centrifuge.SubscribeReply{}, centrifuge.ErrorPermissionDenied)
		return
	}
	options := centrifuge.SubscribeOptions{EmitPresence: true, EmitJoinLeave: true, PushJoinLeave: true, ChannelInfo: each.viewer.Info}
	if stream {
		options = centrifuge.SubscribeOptions{EnablePositioning: true, EnableRecovery: true, RecoveryMode: centrifuge.RecoveryModeStream}
	}
	s.watch(each, event.Channel)
	done(centrifuge.SubscribeReply{Options: options}, nil)
}

func (s *Server) presence(each *conn, event centrifuge.PresenceEvent, done centrifuge.PresenceCallback) {
	room, stream, ok := channelRoom(event.Channel)
	if !ok || stream || s.backend.Authorize(each.viewer, room) != nil {
		done(centrifuge.PresenceReply{}, centrifuge.ErrorPermissionDenied)
		return
	}
	result, err := s.node.Presence(event.Channel)
	done(centrifuge.PresenceReply{Result: &result}, err)
}

// refresh asks again whether the session behind a socket is good, so that signing out ends the socket without waiting for its
// next command.
func (s *Server) refresh(each *conn, done centrifuge.RefreshCallback) {
	if !s.backend.Alive(each.viewer) {
		done(centrifuge.RefreshReply{Expired: true}, nil)
		return
	}
	done(centrifuge.RefreshReply{ExpireAt: s.expiry()}, nil)
}

func (s *Server) add(each *conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[each.client.ID()] = each
}

func (s *Server) watch(each *conn, channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channel[channel] == nil {
		s.channel[channel] = map[string]*conn{}
	}
	s.channel[channel][each.client.ID()] = each
	each.channels[channel] = struct{}{}
}

func (s *Server) unsubscribed(each *conn, channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unwatchLocked(each, channel)
}

func (s *Server) unwatchLocked(each *conn, channel string) {
	watching := s.channel[channel]
	delete(each.channels, channel)
	delete(watching, each.client.ID())
	if len(watching) == 0 {
		delete(s.channel, channel)
	}
}

func (s *Server) remove(each *conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for channel := range each.channels {
		s.unwatchLocked(each, channel)
	}
	delete(s.conns, each.client.ID())
	s.settleLocked()
}

func (s *Server) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

func (s *Server) snapshot() []*conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := make([]*conn, 0, len(s.conns))
	for _, each := range s.conns {
		all = append(all, each)
	}
	return all
}

// watchers is the sockets subscribed to a channel.
func (s *Server) watchers(channel string) []*conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := make([]*conn, 0, len(s.channel[channel]))
	for _, each := range s.channel[channel] {
		all = append(all, each)
	}
	return all
}

// Revalidate asks again whether each person watching a room is still allowed, and takes the room from those who are not. It
// is called when who is in a room changes.
func (s *Server) Revalidate(room string) {
	for _, channel := range []string{StreamChannel(room), LiveChannel(room)} {
		for _, each := range s.watchers(channel) {
			if s.backend.Authorize(each.viewer, room) != nil {
				each.client.Unsubscribe(channel, centrifuge.Unsubscribe{Code: unsubscribeLeftRoom, Reason: "not in this room"})
			}
		}
	}
}

// restart sends the watchers of a room away to subscribe again, because its stream cannot be followed from where they are: the
// room is gone, or its log started again. They come back by recovery, which fails, and the page fetches a snapshot.
func (s *Server) restart(room string) {
	for _, channel := range []string{StreamChannel(room), LiveChannel(room)} {
		for _, each := range s.watchers(channel) {
			each.client.Unsubscribe(channel, centrifuge.Unsubscribe{Code: centrifuge.UnsubscribeCodeInsufficient, Reason: "the room's stream started again"})
		}
	}
}
