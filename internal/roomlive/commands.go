package roomlive

import (
	"context"
	"encoding/json"
	"time"

	"github.com/centrifugal/centrifuge"
)

// commandTimeout bounds how long a command may be carried out.
const commandTimeout = 30 * time.Second

// maxID is the longest id a browser may give a command.
const maxID = 64

// command carries out what a browser sent as an RPC: say, stop, seen or typing. Typing is ephemeral and answered here. The
// others are carried out by the Backend as the person, after the person is checked against the room again, and at most once
// for an id.
func (s *Server) command(each *conn, event centrifuge.RPCEvent, done centrifuge.RPCCallback) {
	command, err := decodeCommand(event)
	if err != nil {
		done(centrifuge.RPCReply{}, err)
		return
	}
	if err := s.backend.Authorize(each.viewer, command.Room); err != nil {
		done(centrifuge.RPCReply{}, centrifuge.ErrorPermissionDenied)
		return
	}
	if command.Name == CmdTyping {
		s.typing(each, command, done)
		return
	}
	if !each.rate.allow() {
		done(centrifuge.RPCReply{}, centrifuge.ErrorTooManyRequests)
		return
	}
	select {
	case each.slots <- struct{}{}:
	default:
		done(centrifuge.RPCReply{}, centrifuge.ErrorTooManyRequests)
		return
	}
	if !s.begin() {
		<-each.slots
		done(centrifuge.RPCReply{}, centrifuge.ErrorNotAvailable)
		return
	}
	go func() {
		defer s.finish()
		defer func() { <-each.slots }()
		reply(done, s.once(each, command))
	}()
}

// begin notes a command starting, unless the server is shutting down.
func (s *Server) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing.Load() {
		return false
	}
	s.inflight++
	return true
}

func (s *Server) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inflight--
	s.settleLocked()
}

func decodeCommand(event centrifuge.RPCEvent) (Command, error) {
	switch event.Method {
	case CmdSay, CmdStop, CmdSeen, CmdTyping:
	default:
		return Command{}, centrifuge.ErrorMethodNotFound
	}
	var command Command
	if err := json.Unmarshal(event.Data, &command); err != nil || command.Room == "" || len(command.ID) == 0 || len(command.ID) > maxID {
		return Command{}, centrifuge.ErrorBadRequest
	}
	command.Name = event.Method
	return command, nil
}

// once carries a command out unless this person already sent it under this id.
func (s *Server) once(each *conn, command Command) Result {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	s.commands.Add(1)
	key := each.viewer.Who + "\x00" + command.ID
	result, repeated := s.dedupe.do(ctx, key, func() Result {
		result, err := s.backend.Do(ctx, each.viewer, command)
		if err != nil {
			return Result{Error: err.Error()}
		}
		return result
	})
	if repeated {
		s.repeated.Add(1)
	}
	return result
}

func reply(done centrifuge.RPCCallback, result Result) {
	data, err := json.Marshal(result)
	done(centrifuge.RPCReply{Data: data}, err)
}

// typing tells a room that somebody is writing. It is not logged and not stored: it is published to the room's live channel,
// and a socket that sends more than one every typingEvery for a room is not passed on.
func (s *Server) typing(each *conn, command Command, done centrifuge.RPCCallback) {
	if each.typed.pass(command.Room, time.Now()) {
		data, _ := json.Marshal(Typing{V: Version, Type: FrameTyping, Room: command.Room, Thread: command.Thread, Who: each.viewer.Who, Name: each.viewer.Name})
		if _, err := s.node.Publish(LiveChannel(command.Room), data); err != nil {
			done(centrifuge.RPCReply{}, err)
			return
		}
	}
	reply(done, Result{OK: true})
}
