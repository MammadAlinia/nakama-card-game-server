package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/heroiclabs/nakama-common/runtime"
)

type commandTestMessage struct {
	matchTestPresence
	opCode int64
	data   []byte
}

func (m commandTestMessage) GetOpCode() int64      { return m.opCode }
func (m commandTestMessage) GetData() []byte       { return m.data }
func (m commandTestMessage) GetReliable() bool     { return true }
func (m commandTestMessage) GetReceiveTime() int64 { return 0 }

// Nakama's logger formats its message like fmt.Printf.
type commandTestLogger struct {
	runtime.Logger
	warnings []string
}

func (l *commandTestLogger) Warn(format string, args ...interface{}) {
	l.warnings = append(l.warnings, fmt.Sprintf(format, args...))
}

func TestMatchLoopStartAdmission(t *testing.T) {
	owner := matchTestPresence{"owner", "session-one"}
	tests := []struct {
		name      string
		message   runtime.MatchData
		joined    bool
		reserved  string
		wantStart bool
	}{
		{"joined owner starts", commandTestMessage{owner, 1, nil}, true, "session-one", true},
		{"empty byte payload is allowed", commandTestMessage{owner, 1, []byte{}}, true, "session-one", true},
		{"other user ignored", commandTestMessage{matchTestPresence{"other-user", "session-one"}, 1, nil}, true, "session-one", false},
		{"other session ignored", commandTestMessage{matchTestPresence{"owner", "session-two"}, 1, nil}, true, "session-one", false},
		{"empty session ignored", commandTestMessage{matchTestPresence{"owner", ""}, 1, nil}, true, "session-one", false},
		{"unreserved session ignored", commandTestMessage{owner, 1, nil}, true, "", false},
		{"unjoined player ignored", commandTestMessage{owner, 1, nil}, false, "session-one", false},
		{"unknown opcode ignored", commandTestMessage{owner, 99, nil}, true, "session-one", false},
		{"nil message ignored", nil, true, "session-one", false},
		{"nonempty Start payload rejected", commandTestMessage{owner, 1, []byte(`{}`)}, true, "session-one", false},
		{"payload cannot establish ownership", commandTestMessage{matchTestPresence{"other-user", "session-one"}, 1, []byte(`{"owner_user_id":"owner"}`)}, true, "session-one", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &BlackjackMatchState{
				joined: tt.joined, ownerUserID: "owner", playerSessionID: tt.reserved,
			}
			if tt.joined {
				state.playerPresence = matchTestPresence{"owner", tt.reserved}
			}
			before := *state
			match := &BlackjackMatch{}
			logger := &commandTestLogger{}
			got := match.MatchLoop(context.Background(), logger, nil, nil, nil, 10, state, []runtime.MatchData{tt.message})
			if got != state {
				t.Errorf("message handling should retain the match state; got %T", got)
			}
			if tt.wantStart {
				if state.round == nil {
					t.Fatal("authorized Start did not create a round")
				}
				if len(state.round.playerHand) != 2 || len(state.round.dealerHand) != 2 || state.round.deck == nil || len(state.round.deck.cards) != 48 {
					t.Error("authorized Start should deal two cards each from a fresh deck")
				}
				before.round = state.round
			}
			if *state != before {
				t.Errorf("unexpected state change: got %+v; want %+v", *state, before)
			}
		})
	}
}

func TestMatchLoopStartAfterIgnoredMessages(t *testing.T) {
	owner := matchTestPresence{"owner", "session-one"}
	state := &BlackjackMatchState{
		joined: true, ownerUserID: "owner", playerSessionID: "session-one", playerPresence: owner,
	}
	messages := []runtime.MatchData{
		nil,
		commandTestMessage{matchTestPresence{"other-user", "session-one"}, 1, nil},
		commandTestMessage{matchTestPresence{"owner", "session-two"}, 1, nil},
		commandTestMessage{owner, 99, nil},
		commandTestMessage{owner, 1, nil},
	}
	match := &BlackjackMatch{}
	got := match.MatchLoop(context.Background(), &commandTestLogger{}, nil, nil, nil, 10, state, messages)
	if got != state || state.round == nil {
		t.Fatal("ignored messages must not prevent a later valid Start in the same batch")
	}
}

func TestMatchLoopRejectedStartsPreserveActiveRound(t *testing.T) {
	for _, phase := range []Phase{PhasePlayerTurn, PhaseDealerTurn} {
		t.Run(string(phase), func(t *testing.T) {
			round, err := NewRound(NewDeck())
			if err != nil {
				t.Fatalf("round setup failed: %v", err)
			}
			round.phase = phase
			owner := matchTestPresence{"owner", "session-one"}
			state := &BlackjackMatchState{
				round: round, joined: true, ownerUserID: "owner", playerSessionID: "session-one", playerPresence: owner,
			}
			before := *state
			beforeDeck := round.deck
			beforePlayer := slices.Clone(round.playerHand)
			beforeDealer := slices.Clone(round.dealerHand)
			beforeCards := slices.Clone(round.deck.cards)
			beforeOutcome := round.outcome
			logger := &commandTestLogger{}
			match := &BlackjackMatch{}
			messages := []runtime.MatchData{commandTestMessage{owner, 1, nil}, commandTestMessage{owner, 1, nil}}
			got := match.MatchLoop(context.Background(), logger, nil, nil, nil, 10, state, messages)
			if got != state || *state != before {
				t.Fatal("rejected Starts should preserve the match and its active round")
			}
			if round.phase != phase || round.outcome != beforeOutcome || round.deck != beforeDeck || !slices.Equal(round.playerHand, beforePlayer) || !slices.Equal(round.dealerHand, beforeDealer) || !slices.Equal(round.deck.cards, beforeCards) {
				t.Error("rejected Starts changed the active round")
			}
			if len(logger.warnings) != 2 {
				t.Fatalf("both rejected Starts should be processed and logged; got %d warnings", len(logger.warnings))
			}
			for _, warning := range logger.warnings {
				if strings.Contains(warning, "%!") || !strings.Contains(warning, "active round") {
					t.Errorf("warning should format the rejection error correctly; got %q", warning)
				}
			}
		})
	}
}

func TestMatchLoopHitAndStand(t *testing.T) {
	tests := []struct {
		name        string
		opCode      int64
		player      [2]int
		dealer      [2]int
		draws       []int
		playerDraws int
		dealerDraws int
		wantPhase   Phase
		wantOutcome Outcome
	}{
		{"Hit draws one card", 2, [2]int{10, 7}, [2]int{9, 8}, []int{2}, 1, 0, PhasePlayerTurn, OutcomeContinue},
		{"Hit bust leaves dealer untouched", 2, [2]int{10, 7}, [2]int{10, 6}, []int{5, 2}, 1, 0, PhaseFinished, OutcomeLoss},
		{"Hit to twenty-one resolves dealer", 2, [2]int{10, 7}, [2]int{10, 6}, []int{4, 5}, 1, 1, PhaseFinished, OutcomeDraw},
		{"Stand draws dealer cards", 3, [2]int{10, 9}, [2]int{10, 6}, []int{2}, 0, 1, PhaseFinished, OutcomeWon},
		{"Stand respects dealer seventeen", 3, [2]int{10, 7}, [2]int{10, 7}, nil, 0, 0, PhaseFinished, OutcomeDraw},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			round, drawCards, remaining := newActionTestRound(t, tt.player, tt.dealer, tt.draws)
			wantPlayer := append(slices.Clone(round.playerHand), drawCards[:tt.playerDraws]...)
			consumed := tt.playerDraws + tt.dealerDraws
			wantDealer := append(slices.Clone(round.dealerHand), drawCards[tt.playerDraws:consumed]...)
			wantRemaining := append(slices.Clone(drawCards[consumed:]), remaining)
			owner := matchTestPresence{"owner", "session-one"}
			state := &BlackjackMatchState{
				round: round, joined: true, ownerUserID: "owner", playerSessionID: "session-one", playerPresence: owner,
			}
			before := *state
			logger := &commandTestLogger{}
			match := &BlackjackMatch{}
			got := match.MatchLoop(context.Background(), logger, nil, nil, nil, 10, state, []runtime.MatchData{commandTestMessage{owner, tt.opCode, nil}})
			if got != state || *state != before {
				t.Fatal("a valid action should retain the match, player information, and round pointer")
			}
			if !slices.Equal(round.playerHand, wantPlayer) || !slices.Equal(round.dealerHand, wantDealer) || !slices.Equal(round.deck.cards, wantRemaining) {
				t.Errorf("unexpected cards: player=%v, dealer=%v, remaining=%v; want %v, %v, %v",
					round.playerHand, round.dealerHand, round.deck.cards, wantPlayer, wantDealer, wantRemaining)
			}
			if round.phase != tt.wantPhase || round.outcome != tt.wantOutcome {
				t.Errorf("phase=%q, outcome=%q; want %q, %q", round.phase, round.outcome, tt.wantPhase, tt.wantOutcome)
			}
			if len(logger.warnings) != 0 {
				t.Errorf("successful actions, including a bust, should not log rejection warnings: %v", logger.warnings)
			}
		})
	}
}

func TestMatchLoopHitAndStandRejectionsPreserveState(t *testing.T) {
	tests := []struct {
		name       string
		presence   matchTestPresence
		data       []byte
		unjoined   bool
		unreserved bool
		noRound    bool
		phase      Phase
	}{
		{name: "other user", presence: matchTestPresence{"other-user", "session-one"}},
		{name: "other session", presence: matchTestPresence{"owner", "session-two"}},
		{name: "empty session", presence: matchTestPresence{"owner", ""}},
		{name: "unreserved session", presence: matchTestPresence{"owner", "session-one"}, unreserved: true},
		{name: "not joined", presence: matchTestPresence{"owner", "session-one"}, unjoined: true},
		{name: "nonempty payload", presence: matchTestPresence{"owner", "session-one"}, data: []byte(`{}`)},
		{name: "before Start", presence: matchTestPresence{"owner", "session-one"}, noRound: true},
		{name: "finished round", presence: matchTestPresence{"owner", "session-one"}, phase: PhaseFinished},
		{name: "dealer turn", presence: matchTestPresence{"owner", "session-one"}, phase: PhaseDealerTurn},
	}
	for _, opCode := range []int64{2, 3} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("opcode %d/%s", opCode, tt.name), func(t *testing.T) {
				state := &BlackjackMatchState{
					joined: !tt.unjoined, ownerUserID: "owner", playerSessionID: "session-one",
					playerPresence: matchTestPresence{"owner", "session-one"},
				}
				if tt.unreserved {
					state.playerSessionID = ""
				}
				if !tt.noRound {
					round, _, _ := newActionTestRound(t, [2]int{10, 7}, [2]int{10, 6}, []int{2})
					if tt.phase != "" {
						round.phase = tt.phase
					}
					if tt.phase == PhaseFinished {
						round.outcome = OutcomeLoss
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
				got := match.MatchLoop(context.Background(), &commandTestLogger{}, nil, nil, nil, 10, state,
					[]runtime.MatchData{commandTestMessage{tt.presence, opCode, tt.data}})
				if got != state || *state != before {
					t.Fatal("a rejected action should preserve the match state")
				}
				if state.round != nil {
					round := state.round
					if round.phase != beforeRound.phase || round.outcome != beforeRound.outcome || round.deck != beforeRound.deck ||
						!slices.Equal(round.playerHand, beforePlayer) || !slices.Equal(round.dealerHand, beforeDealer) || !slices.Equal(round.deck.cards, beforeCards) {
						t.Error("a rejected action changed the round or consumed cards")
					}
				}
			})
		}
	}
}

func TestMatchLoopHitThenStandBatch(t *testing.T) {
	round, drawCards, remaining := newActionTestRound(t, [2]int{10, 6}, [2]int{9, 7}, []int{3, 2})
	wantPlayer := append(slices.Clone(round.playerHand), drawCards[0])
	wantDealer := append(slices.Clone(round.dealerHand), drawCards[1])
	owner := matchTestPresence{"owner", "session-one"}
	state := &BlackjackMatchState{
		round: round, joined: true, ownerUserID: "owner", playerSessionID: "session-one", playerPresence: owner,
	}
	messages := []runtime.MatchData{
		commandTestMessage{owner, 2, nil}, // Player draws to nineteen.
		commandTestMessage{owner, 3, nil}, // Dealer draws to eighteen; player wins.
		commandTestMessage{owner, 2, nil}, // Finished round must reject another Hit.
	}
	logger := &commandTestLogger{}
	match := &BlackjackMatch{}
	got := match.MatchLoop(context.Background(), logger, nil, nil, nil, 10, state, messages)
	if got != state || state.round != round {
		t.Fatal("processing a batch should retain the match and round")
	}
	assertCompletedAction(t, round, wantPlayer, wantDealer, []Card{remaining}, OutcomeWon)
	if len(logger.warnings) != 1 {
		t.Errorf("only the Hit after completion should be rejected; got warnings %v", logger.warnings)
	}
}
