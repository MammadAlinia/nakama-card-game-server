package main

import (
	"context"
	"slices"
	"testing"

	"github.com/heroiclabs/nakama-common/runtime"
)

func TestMatchInitWaitingState(t *testing.T) {
	match := &BlackjackMatch{}
	params := map[string]interface{}{"owner_user_id": "owner"}
	value, tickRate, label := match.MatchInit(context.Background(), nil, nil, nil, params)

	state, ok := value.(*BlackjackMatchState)
	if !ok || state == nil {
		t.Fatalf("MatchInit() returned %T; want a non-nil *BlackjackMatchState", value)
	}
	if state.round != nil {
		t.Error("a new match should wait for Start without creating a round")
	}
	if state.joined {
		t.Error("a new match should start without a joined player")
	}
	if state.ownerUserID != "owner" {
		t.Errorf("owner user ID = %q; want owner", state.ownerUserID)
	}
	if state.playerSessionID != "" {
		t.Error("initialization should not reserve a player session")
	}
	if state.playerPresence != nil {
		t.Error("initialization should not store a joined player's presence")
	}
	if tickRate != 5 {
		t.Errorf("tick rate = %d; want 5", tickRate)
	}
	if label != "blackjack" {
		t.Errorf("label = %q; want blackjack", label)
	}
}

func TestMatchInitIndependentStates(t *testing.T) {
	match := &BlackjackMatch{}
	firstValue, _, _ := match.MatchInit(context.Background(), nil, nil, nil, map[string]interface{}{"owner_user_id": "first-owner"})
	secondValue, _, _ := match.MatchInit(context.Background(), nil, nil, nil, map[string]interface{}{"owner_user_id": "second-owner"})

	first, ok := firstValue.(*BlackjackMatchState)
	if !ok || first == nil {
		t.Fatal("first initialization did not return a non-nil *BlackjackMatchState")
	}
	second, ok := secondValue.(*BlackjackMatchState)
	if !ok || second == nil {
		t.Fatal("second initialization did not return a non-nil *BlackjackMatchState")
	}
	if first == second {
		t.Fatal("separate matches should not share the same state pointer")
	}

	// Changing one match must not create a round in another match.
	first.round = &Round{}
	if second.round != nil {
		t.Error("changing the first match affected the second match's round")
	}
	if first.ownerUserID != "first-owner" || second.ownerUserID != "second-owner" {
		t.Error("each match should retain its own owner user ID")
	}
}

func TestMatchInitInvalidOwner(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{"nil parameters", nil},
		{"missing owner", map[string]interface{}{}},
		{"nil owner", map[string]interface{}{"owner_user_id": nil}},
		{"wrong owner type", map[string]interface{}{"owner_user_id": 42}},
		{"empty owner", map[string]interface{}{"owner_user_id": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := &BlackjackMatch{}
			state, _, _ := match.MatchInit(context.Background(), nil, nil, nil, tt.params)
			if state != nil {
				t.Error("MatchInit() should return nil when the owner is missing or invalid")
			}
		})
	}
}

func TestMatchLoopTimeout(t *testing.T) {
	tests := []struct {
		name    string
		joined  bool
		tick    int64
		wantEnd bool
	}{
		{"unjoined at creation", false, 0, false},
		{"unjoined before timeout", false, 299, false},
		{"unjoined at timeout", false, 300, true},
		{"unjoined after timeout", false, 301, true},
		{"joined at timeout", true, 300, false},
		{"joined after timeout", true, 1000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &BlackjackMatchState{joined: tt.joined}
			if tt.joined {
				state.round = &Round{phase: PhasePlayerTurn, outcome: OutcomeContinue}
			}
			beforeRound := state.round
			match := &BlackjackMatch{}
			got := match.MatchLoop(context.Background(), nil, nil, nil, nil, tt.tick, state, nil)

			if tt.wantEnd {
				if got != nil {
					t.Errorf("MatchLoop() returned %T; want nil to end an unjoined match", got)
				}
				return
			}
			if got != state {
				t.Errorf("MatchLoop() should return the existing state pointer; got %T", got)
			}
			if state.round != beforeRound || state.joined != tt.joined {
				t.Error("an idle loop changed the round or joined status")
			}
		})
	}
}

func TestMatchLoopInvalidState(t *testing.T) {
	tests := []struct {
		name  string
		state interface{}
	}{
		{"nil interface", nil},
		{"nil state pointer", (*BlackjackMatchState)(nil)},
		{"wrong state type", "invalid state"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := &BlackjackMatch{}
			got := match.MatchLoop(context.Background(), nil, nil, nil, nil, 0, tt.state, nil)
			if got != nil {
				t.Errorf("MatchLoop() returned %T; want nil to end a match with invalid state", got)
			}
		})
	}
}

// These identities stand in for the Presence Nakama supplies to a join callback.
type matchTestPresence struct {
	userID    string
	sessionID string
}

func (p matchTestPresence) GetUserId() string    { return p.userID }
func (p matchTestPresence) GetSessionId() string { return p.sessionID }
func (p matchTestPresence) GetNodeId() string    { return "test-node" }
func (p matchTestPresence) GetHidden() bool      { return false }
func (p matchTestPresence) GetPersistence() bool { return false }
func (p matchTestPresence) GetUsername() string  { return "test-player" }
func (p matchTestPresence) GetStatus() string    { return "" }
func (p matchTestPresence) GetReason() runtime.PresenceReason {
	return runtime.PresenceReasonUnknown
}

func TestMatchJoinAttemptAdmission(t *testing.T) {
	tests := []struct {
		name       string
		presence   runtime.Presence
		reserved   string
		joined     bool
		tick       int64
		metadata   map[string]string
		wantAccept bool
		wantEnd    bool
	}{
		{name: "owner reserves first session", presence: matchTestPresence{"owner", "session-one"}, wantAccept: true},
		{name: "owner before timeout", presence: matchTestPresence{"owner", "session-one"}, tick: 299, wantAccept: true},
		{name: "other user rejected", presence: matchTestPresence{"other-user", "session-one"}},
		{name: "metadata cannot grant ownership", presence: matchTestPresence{"other-user", "session-one"}, metadata: map[string]string{"owner_user_id": "other-user"}},
		{name: "nil presence rejected", presence: nil},
		{name: "empty session rejected", presence: matchTestPresence{"owner", ""}},
		{name: "same reserved session may retry", presence: matchTestPresence{"owner", "session-one"}, reserved: "session-one", wantAccept: true},
		{name: "different owner session rejected", presence: matchTestPresence{"owner", "session-two"}, reserved: "session-one"},
		{name: "unjoined at timeout", presence: matchTestPresence{"owner", "session-one"}, tick: 300, wantEnd: true},
		{name: "unjoined after timeout", presence: matchTestPresence{"owner", "session-one"}, tick: 301, wantEnd: true},
		{name: "joined session retry after timeout", presence: matchTestPresence{"owner", "session-one"}, reserved: "session-one", joined: true, tick: 1000, wantAccept: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Report a nil-presence panic as a failure without stopping the other cases.
			defer func() {
				if panicValue := recover(); panicValue != nil {
					t.Fatalf("MatchJoinAttempt() panicked: %v", panicValue)
				}
			}()
			state := &BlackjackMatchState{
				ownerUserID: "owner", playerSessionID: tt.reserved, joined: tt.joined,
			}
			wantState := *state
			if tt.wantAccept {
				wantState.playerSessionID = tt.presence.GetSessionId()
			}
			match := &BlackjackMatch{}
			got, accepted, reason := match.MatchJoinAttempt(context.Background(), nil, nil, nil, nil, tt.tick, state, tt.presence, tt.metadata)
			if accepted != tt.wantAccept {
				t.Errorf("accepted = %t; want %t", accepted, tt.wantAccept)
			}
			if tt.wantEnd {
				if got != nil {
					t.Error("an expired match should return nil state")
				}
			} else if got != state {
				t.Errorf("join admission should retain the existing state pointer; got %T", got)
			}
			if tt.wantAccept && reason != "" {
				t.Errorf("accepted joins should have an empty reason; got %q", reason)
			}
			if !tt.wantAccept && reason == "" {
				t.Error("rejected joins should have a rejection reason")
			}
			if *state != wantState {
				t.Errorf("join admission changed unexpected fields: got %+v; want %+v", *state, wantState)
			}
		})
	}
}

func TestMatchJoinAttemptInvalidState(t *testing.T) {
	tests := []struct {
		name  string
		state interface{}
	}{
		{"nil interface", nil},
		{"nil state pointer", (*BlackjackMatchState)(nil)},
		{"wrong state type", "invalid state"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := &BlackjackMatch{}
			got, accepted, reason := match.MatchJoinAttempt(context.Background(), nil, nil, nil, nil, 0, tt.state, matchTestPresence{"owner", "session-one"}, nil)
			if got != nil || accepted || reason == "" {
				t.Errorf("invalid state should end the match and reject the join; got %T, %t, %q", got, accepted, reason)
			}
		})
	}
}

func TestMatchJoinConfirmation(t *testing.T) {
	owner := matchTestPresence{"owner", "session-one"}
	other := matchTestPresence{"other-user", "session-one"}
	tests := []struct {
		name         string
		presences    []runtime.Presence
		reserved     string
		wantPresence runtime.Presence
	}{
		{"matching owner and session", []runtime.Presence{owner}, "session-one", owner},
		{"empty list", nil, "session-one", nil},
		{"nil entry", []runtime.Presence{nil}, "session-one", nil},
		{"wrong user", []runtime.Presence{other}, "session-one", nil},
		{"wrong session", []runtime.Presence{matchTestPresence{"owner", "session-two"}}, "session-one", nil},
		{"empty session", []runtime.Presence{matchTestPresence{"owner", ""}}, "", nil},
		{"session was not reserved", []runtime.Presence{owner}, "", nil},
		{"valid entry after unrelated and nil entries", []runtime.Presence{other, nil, owner}, "session-one", owner},
		{"valid entry before unrelated entry", []runtime.Presence{owner, other}, "session-one", owner},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if panicValue := recover(); panicValue != nil {
					t.Fatalf("MatchJoin() panicked: %v", panicValue)
				}
			}()
			round := &Round{phase: PhasePlayerTurn, outcome: OutcomeContinue}
			state := &BlackjackMatchState{
				round: round, ownerUserID: "owner", playerSessionID: tt.reserved,
			}
			wantState := *state
			if tt.wantPresence != nil {
				wantState.joined = true
				wantState.playerPresence = tt.wantPresence
			}
			match := &BlackjackMatch{}
			got := match.MatchJoin(context.Background(), nil, nil, nil, nil, 1, state, tt.presences)
			if got != state {
				t.Errorf("MatchJoin() should retain the existing state pointer; got %T", got)
			}
			if *state != wantState {
				t.Errorf("confirmed state = %+v; want %+v", *state, wantState)
			}
			if state.round != round || round.phase != PhasePlayerTurn || round.outcome != OutcomeContinue {
				t.Error("join confirmation changed the existing round")
			}
		})
	}
}

func TestMatchJoinInvalidState(t *testing.T) {
	tests := []struct {
		name  string
		state interface{}
	}{
		{"nil interface", nil},
		{"nil state pointer", (*BlackjackMatchState)(nil)},
		{"wrong state type", "invalid state"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := &BlackjackMatch{}
			got := match.MatchJoin(context.Background(), nil, nil, nil, nil, 0, tt.state, nil)
			if got != nil {
				t.Errorf("MatchJoin() returned %T; want nil for invalid state", got)
			}
		})
	}
}

func TestConfirmedJoinSurvivesTimeout(t *testing.T) {
	match := &BlackjackMatch{}
	value, _, _ := match.MatchInit(context.Background(), nil, nil, nil, map[string]interface{}{"owner_user_id": "owner"})
	state, ok := value.(*BlackjackMatchState)
	if !ok || state == nil {
		t.Fatal("initialization should create a valid match state")
	}
	presence := matchTestPresence{"owner", "session-one"}
	admittedState, accepted, reason := match.MatchJoinAttempt(context.Background(), nil, nil, nil, nil, 299, state, presence, nil)
	if admittedState != state || !accepted || reason != "" {
		t.Fatalf("owner should be admitted before the timeout; got %T, %t, %q", admittedState, accepted, reason)
	}
	if state.joined || state.playerPresence != nil {
		t.Fatal("admission should reserve the session without confirming a join")
	}
	confirmedState := match.MatchJoin(context.Background(), nil, nil, nil, nil, 299, state, []runtime.Presence{presence})
	if confirmedState != state || !state.joined || state.playerPresence != presence {
		t.Fatal("MatchJoin() should confirm and retain the admitted owner's presence")
	}
	if state.round != nil {
		t.Error("joining should not start a blackjack round")
	}
	if got := match.MatchLoop(context.Background(), nil, nil, nil, nil, 1000, state, nil); got != state {
		t.Errorf("a confirmed join should survive the unjoined timeout; got %T", got)
	}
}

func TestMatchLeaveEvents(t *testing.T) {
	owner := matchTestPresence{"owner", "session-one"}
	other := matchTestPresence{"other-user", "session-one"}
	tests := []struct {
		name      string
		presences []runtime.Presence
		reserved  string
		wantEnd   bool
	}{
		{"owner session leaves", []runtime.Presence{owner}, "session-one", true},
		{"empty list", nil, "session-one", false},
		{"nil entry", []runtime.Presence{nil}, "session-one", false},
		{"other user leaves", []runtime.Presence{other}, "session-one", false},
		{"other owner session leaves", []runtime.Presence{matchTestPresence{"owner", "session-two"}}, "session-one", false},
		{"empty session", []runtime.Presence{matchTestPresence{"owner", ""}}, "", false},
		{"session was not reserved", []runtime.Presence{owner}, "", false},
		{"owner after unrelated and nil entries", []runtime.Presence{other, nil, owner}, "session-one", true},
		{"owner before unrelated entry", []runtime.Presence{owner, other}, "session-one", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if panicValue := recover(); panicValue != nil {
					t.Fatalf("MatchLeave() panicked: %v", panicValue)
				}
			}()
			round := &Round{phase: PhasePlayerTurn, outcome: OutcomeContinue}
			state := &BlackjackMatchState{
				round: round, joined: true, ownerUserID: "owner",
				playerSessionID: tt.reserved,
				playerPresence:  matchTestPresence{"owner", tt.reserved},
			}
			before := *state
			match := &BlackjackMatch{}
			got := match.MatchLeave(context.Background(), nil, nil, nil, nil, 10, state, tt.presences)
			if tt.wantEnd {
				if got != nil {
					t.Errorf("the owner's leaving session should end the match; got %T", got)
				}
				return
			}
			if got != state {
				t.Errorf("an unrelated leave event should return the existing state; got %T", got)
			}
			if *state != before {
				t.Errorf("an unrelated leave event changed state: got %+v; want %+v", *state, before)
			}
			if round.phase != PhasePlayerTurn || round.outcome != OutcomeContinue {
				t.Error("an unrelated leave event changed the round")
			}
		})
	}
}

func TestMatchLeaveInvalidState(t *testing.T) {
	tests := []struct {
		name  string
		state interface{}
	}{
		{"nil interface", nil},
		{"nil state pointer", (*BlackjackMatchState)(nil)},
		{"wrong state type", "invalid state"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := &BlackjackMatch{}
			got := match.MatchLeave(context.Background(), nil, nil, nil, nil, 0, tt.state, nil)
			if got != nil {
				t.Errorf("MatchLeave() returned %T; want nil for invalid state", got)
			}
		})
	}
}

func TestMatchTerminateEndsActiveMatch(t *testing.T) {
	round, err := NewRound(NewDeck())
	if err != nil {
		t.Fatalf("round setup failed: %v", err)
	}
	state := &BlackjackMatchState{
		round: round, joined: true, ownerUserID: "owner", playerSessionID: "session-one",
		playerPresence: matchTestPresence{"owner", "session-one"},
	}
	match := &BlackjackMatch{}
	got := match.MatchTerminate(context.Background(), nil, nil, nil, nil, 10, state, 5)
	if got != nil {
		t.Errorf("graceful shutdown should end an active match immediately; got %T", got)
	}
}

func TestMatchSignalPreservesState(t *testing.T) {
	tests := []struct {
		name   string
		data   string
		active bool
	}{
		{"waiting match ignores Start signal", "Start", false},
		{"active match ignores Hit signal", "Hit", true},
		{"active match ignores Stand signal", "Stand", true},
		{"active match ignores empty signal", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &BlackjackMatchState{
				joined: true, ownerUserID: "owner", playerSessionID: "session-one",
				playerPresence: matchTestPresence{"owner", "session-one"},
			}
			if tt.active {
				round, err := NewRound(NewDeck())
				if err != nil {
					t.Fatalf("round setup failed: %v", err)
				}
				state.round = round
			}
			before := *state
			var beforeRound Round
			var beforePlayer, beforeDealer, beforeCards []Card
			if state.round != nil {
				beforeRound = *state.round
				beforePlayer = slices.Clone(state.round.playerHand)
				beforeDealer = slices.Clone(state.round.dealerHand)
				beforeCards = slices.Clone(state.round.deck.cards)
			}
			match := &BlackjackMatch{}
			got, response := match.MatchSignal(context.Background(), nil, nil, nil, nil, 10, state, tt.data)
			if got != state || response != "unsupported signal" {
				t.Errorf("unsupported signal should retain the state and report unsupported signal; got %T, %q", got, response)
			}
			if *state != before {
				t.Fatal("a signal changed the round pointer or player information")
			}
			if state.round != nil {
				round := state.round
				if round.phase != beforeRound.phase || round.outcome != beforeRound.outcome || round.deck != beforeRound.deck {
					t.Error("a signal changed the round's phase, outcome, or deck")
				}
				if !slices.Equal(round.playerHand, beforePlayer) || !slices.Equal(round.dealerHand, beforeDealer) || !slices.Equal(round.deck.cards, beforeCards) {
					t.Error("a signal changed a hand or drew cards from the deck")
				}
			}
		})
	}
}

func TestMatchSignalInvalidState(t *testing.T) {
	tests := []struct {
		name  string
		state interface{}
	}{
		{"nil interface", nil},
		{"nil state pointer", (*BlackjackMatchState)(nil)},
		{"wrong state type", "invalid state"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := &BlackjackMatch{}
			got, response := match.MatchSignal(context.Background(), nil, nil, nil, nil, 0, tt.state, "Hit")
			if got != nil || response != "invalid state" {
				t.Errorf("invalid state should end the match and report invalid state; got %T, %q", got, response)
			}
		})
	}
}
