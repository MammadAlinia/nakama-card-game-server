package main

import (
	"context"
	"database/sql"
	"errors"

	"github.com/heroiclabs/nakama-common/runtime"
)

type BlackjackMatch struct{}

type BlackjackMatchState struct {
	round           *Round
	joined          bool
	ownerUserID     string
	playerSessionID string
	playerPresence  runtime.Presence
}

var _ runtime.Match = (*BlackjackMatch)(nil)

func (m *BlackjackMatch) MatchInit(
	ctx context.Context,
	logger runtime.Logger,
	db *sql.DB,
	nk runtime.NakamaModule,
	params map[string]interface{},
) (interface{}, int, string) {
	state := &BlackjackMatchState{}
	tickRate := 5
	label := "blackjack"

	ownerID, ok := params["owner_user_id"].(string)
	if ownerID == "" || !ok {
		return nil, tickRate, "blackjack"
	}

	state.ownerUserID = ownerID

	return state, tickRate, label
}

const (
	opStart    int64 = 1
	opHit      int64 = 2
	opStand    int64 = 3
	opGetState int64 = 4
	opState    int64 = 100
	opError    int64 = 101
)

func (m *BlackjackMatch) MatchLoop(
	ctx context.Context,
	logger runtime.Logger,
	db *sql.DB,
	nk runtime.NakamaModule,
	dispatcher runtime.MatchDispatcher,
	tick int64,
	state interface{},
	messages []runtime.MatchData,
) interface{} {
	match, ok := state.(*BlackjackMatchState)
	if !ok || match == nil {
		return nil
	}
	if !match.joined {
		if tick >= 300 {
			return nil // Expired: end the match.
		}
		return match // Waiting: keep alive, ignore messages.
	}

	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.GetUserId() != match.ownerUserID {
			continue
		}
		if msgSession := msg.GetSessionId(); msgSession == "" || msgSession != match.playerSessionID {
			continue
		}
		var actionErr error
		if len(msg.GetData()) != 0 {
			actionErr = errors.New("commands do not accept a payload")

		} else {

			switch msg.GetOpCode() {
			case opStart:
				actionErr = match.startRound()

			case opHit:

				if match.round == nil { // Match is looping while not started
					actionErr = errors.New("round has not started yet")

				} else {

					actionErr = match.round.Hit()
				}

			case opStand:

				if match.round == nil { // Match is looping while not started
					actionErr = errors.New("round has not started yet")
				} else {

					actionErr = match.round.Stand()
				}

			case opGetState:

			default:
				actionErr = errors.New("unknown command")
			}
		}
		if actionErr != nil {
			logger.Warn(
				"Command %d rejected: %v",
				msg.GetOpCode(),
				actionErr,
			)
			if err := match.sendError(
				dispatcher,
				msg.GetOpCode(),
				actionErr.Error(),
			); err != nil {
				logger.Warn("Error reply delivery failed: %v", err)
			}
			continue
		}

		if err := match.sendSnapshot(dispatcher); err != nil {
			logger.Warn("Snapshot delivery failed: %v", err)
		}
	}
	return match
}

func (m *BlackjackMatch) MatchJoinAttempt(
	ctx context.Context,
	logger runtime.Logger,
	db *sql.DB,
	nk runtime.NakamaModule,
	dispatcher runtime.MatchDispatcher,
	tick int64,
	state interface{},
	presence runtime.Presence,
	metadata map[string]string,
) (interface{}, bool, string) {
	match, ok := state.(*BlackjackMatchState)
	if !ok || match == nil {
		return nil, false, "invalid state"
	}

	if !match.joined && tick >= 300 {
		return nil, false, "match expired"
	}
	if presence == nil {
		return match, false, "missing presence"
	}
	if id := presence.GetUserId(); match.ownerUserID != id {

		return match, false, "invalid user id"
	}
	sessionID := presence.GetSessionId()

	if sessionID == "" {
		return match, false, "missing session ID"
	}
	if match.playerSessionID != "" &&
		match.playerSessionID != sessionID {
		return match, false, "another session is already reserved"
	}
	match.playerSessionID = sessionID
	return match, true, ""
}
func (m *BlackjackMatch) MatchJoin(
	ctx context.Context,
	logger runtime.Logger,
	db *sql.DB,
	nk runtime.NakamaModule,
	dispatcher runtime.MatchDispatcher,
	tick int64,
	state interface{},
	presences []runtime.Presence,
) interface{} {
	match, ok := state.(*BlackjackMatchState)
	if !ok || match == nil {
		return nil
	}

	for _, presence := range presences {
		if presence == nil {
			continue
		}

		sessionID := presence.GetSessionId()
		if presence.GetUserId() == match.ownerUserID &&
			sessionID != "" &&
			sessionID == match.playerSessionID {
			match.playerPresence = presence
			match.joined = true
			if err := match.sendSnapshot(dispatcher); err != nil {
				logger.Warn("Initial snapshot delivery failed: %v", err)
			}
			return match
		}

	}
	return match
}
func (m *BlackjackMatch) MatchLeave(
	ctx context.Context,
	logger runtime.Logger,
	db *sql.DB,
	nk runtime.NakamaModule,
	dispatcher runtime.MatchDispatcher,
	tick int64,
	state interface{},
	presences []runtime.Presence,
) interface{} {
	match, ok := state.(*BlackjackMatchState)
	if !ok || match == nil {
		return nil
	}

	for _, presence := range presences {
		if presence == nil {
			continue
		}

		sessionID := presence.GetSessionId()
		if presence.GetUserId() == match.ownerUserID &&
			sessionID != "" &&
			sessionID == match.playerSessionID {
			return nil
		}
	}
	return state
}
func (m *BlackjackMatch) MatchTerminate(
	ctx context.Context,
	logger runtime.Logger,
	db *sql.DB,
	nk runtime.NakamaModule,
	dispatcher runtime.MatchDispatcher,
	tick int64,
	state interface{},
	graceSeconds int,
) interface{} {
	return nil
}
func (m *BlackjackMatch) MatchSignal(
	ctx context.Context,
	logger runtime.Logger,
	db *sql.DB,
	nk runtime.NakamaModule,
	dispatcher runtime.MatchDispatcher,
	tick int64,
	state interface{},
	data string,
) (interface{}, string) {
	match, ok := state.(*BlackjackMatchState)
	if !ok || match == nil {
		return nil, "invalid state"
	}
	return match, "unsupported signal"
}
func (s *BlackjackMatchState) startRound() error {

	if !s.joined {
		return errors.New(" cannot start a new round without player")
	}
	if s.round != nil && s.round.phase != PhaseFinished {
		return errors.New(" the player has an active round")
	}
	deck := NewDeck()
	err := deck.Shuffle()
	if err != nil {
		return err
	}
	newRound, err := NewRound(deck)
	if err != nil {
		return err
	}
	s.round = newRound
	return nil
}
