// Package roomlive keeps the people looking at a room up to date over one WebSocket each.
//
// It is built on Centrifuge, the embeddable Go server: a Node holds the connections, a channel is a room's stream, and the
// browser speaks Centrifuge's client protocol in JSON. What this package adds is the part that is gusto's: the Broker serves
// a channel from the room's durable event log, so the stream position a client holds is a sequence number in that log and
// replay after a reconnect, a restart or a slow client's disconnect comes from the log; and the commands people send over the
// socket are carried out by the same code as the page's own routes.
//
// A room has two channels. The stream, room:<id>, is durable and positioned: it carries what happened, each turn or note as the
// same JSON the page's snapshot of the conversation gives, built once for everybody, in the order it was logged. The other, live:<id>, is ephemeral: who is here and who is
// typing. Nothing on it is ever logged.
package roomlive

import (
	"encoding/json"
	"strings"
)

// Version is the version of the frames this package sends. A browser that does not know it reloads the page.
const Version = 1

const (
	streamPrefix = "room:"
	livePrefix   = "live:"
)

// StreamChannel is the durable channel of a room's events.
func StreamChannel(room string) string { return streamPrefix + room }

// LiveChannel is the ephemeral channel of a room: presence and typing.
func LiveChannel(room string) string { return livePrefix + room }

// channelRoom splits a channel into its room, and says whether it is a stream or the live channel.
func channelRoom(channel string) (room string, stream, ok bool) {
	if room, found := strings.CutPrefix(channel, streamPrefix); found && room != "" {
		return room, true, true
	}
	if room, found := strings.CutPrefix(channel, livePrefix); found && room != "" {
		return room, false, true
	}
	return "", false, false
}

// The kinds of event in a room's log that the live layer treats differently, as the store names them.
const (
	kindTurn   = "turn"
	kindNote   = "note"
	kindPeople = "people"
)

// The types of frame on a room's stream.
const (
	// FrameTurn and FrameNote carry one turn or note as the page's snapshot gives it, which the page replaces by its id, or adds
	// at the end.
	FrameTurn = "turn"
	FrameNote = "note"
	// FramePeople says who is in the room, or its agents, changed.
	FramePeople = "people"
	// FrameRefresh says the page should fetch the conversation again, because what changed could not be drawn as one piece.
	FrameRefresh = "refresh"
	// FrameSkip stands in for an event whose effect a later one in the same replay carries.
	FrameSkip = "skip"
)

// The types of frame on a room's live channel.
const (
	FrameTyping = "typing"
)

// Frame is a message on a room's stream, as the browser receives it.
type Frame struct {
	V      int    `json:"v"`
	Type   string `json:"type"`
	Room   string `json:"room"`
	Thread string `json:"thread,omitempty"`
	Seq    uint64 `json:"seq"`
	ID     string `json:"id,omitempty"`
	// Turn is the turn or note of a turn or note frame, as JSON.
	Turn json.RawMessage `json:"turn,omitempty"`
	Ver  string          `json:"ver,omitempty"`
}

// encode is the frame as JSON. Only a turn that is not valid JSON fails to encode, and then the frame says to take a snapshot.
func (f Frame) encode() []byte {
	data, err := json.Marshal(f)
	if err != nil {
		// A frame without a turn holds only strings and numbers, so it always encodes.
		data, _ = json.Marshal(Frame{V: f.V, Type: FrameRefresh, Room: f.Room, Thread: f.Thread, Seq: f.Seq})
	}
	return data
}

// Typing is a message on a room's live channel: somebody is writing in a conversation.
type Typing struct {
	V      int    `json:"v"`
	Type   string `json:"type"`
	Room   string `json:"room"`
	Thread string `json:"thread,omitempty"`
	Who    string `json:"who"`
	Name   string `json:"name"`
}

// Command is what a browser sends over the socket. The ID is the browser's own for this command, so that a command sent again
// after a reconnect is carried out once.
type Command struct {
	// Name is the command's name, which the socket's method carries.
	Name   string `json:"-"`
	V      int    `json:"v"`
	ID     string `json:"id"`
	Room   string `json:"room"`
	Thread string `json:"thread,omitempty"`
	// Text is the message of a say command; Ref is the turn a stop command names, or the time a seen command is for.
	Text string `json:"text,omitempty"`
	Ref  string `json:"ref,omitempty"`
	At   int64  `json:"at,omitempty"`
	// Retry says the browser sent this command before and did not hear it answered, so it is looked for among what was already
	// done, including by a server that has restarted since.
	Retry bool `json:"retry,omitempty"`
}

// The commands there are, named as the browser calls them.
const (
	CmdSay    = "say"
	CmdStop   = "stop"
	CmdSeen   = "seen"
	CmdTyping = "typing"
)

// Result is what a command answers: the turn or note it made, and whether it had already been done.
type Result struct {
	OK    bool   `json:"ok"`
	Ref   string `json:"ref,omitempty"`
	Dupe  bool   `json:"dupe,omitempty"`
	Error string `json:"error,omitempty"`
}
