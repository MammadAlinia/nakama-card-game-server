package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/heroiclabs/nakama-common/runtime"
)

func deliveryTestSnapshot(t *testing.T, dispatcher *snapshotTestDispatcher, index int, target runtime.Presence) StateSnapshot {
	t.Helper()
	if index >= len(dispatcher.broadcasts) {
		t.Fatalf("missing broadcast %d; got %d broadcasts", index, len(dispatcher.broadcasts))
	}
	packet := dispatcher.broadcasts[index]
	if packet.opCode != 100 || !packet.reliable || packet.sender != nil || len(packet.targets) != 1 || packet.targets[0] != target {
		t.Fatalf("snapshot must be reliable opcode 100 addressed only to the joined player: %+v", packet)
	}
	var view StateSnapshot
	if err := json.Unmarshal(packet.data, &view); err != nil {
		t.Fatalf("invalid snapshot JSON: %v", err)
	}
	return view
}

func TestMatchJoinDeliversInitialSnapshot(t *testing.T) {
	for _, failDelivery := range []bool{false, true} {
		name := "successful delivery"
		if failDelivery {
			name = "failed delivery preserves membership"
		}
		t.Run(name, func(t *testing.T) {
			match := &BlackjackMatch{}
			value, _, _ := match.MatchInit(context.Background(), nil, nil, nil, map[string]interface{}{"owner_user_id": "owner"})
			state := value.(*BlackjackMatchState)
			owner := matchTestPresence{"owner", "session-one"}
			dispatcher := &snapshotTestDispatcher{}
			if failDelivery {
				dispatcher.err = errors.New("delivery failed")
			}
			_, accepted, reason := match.MatchJoinAttempt(context.Background(), nil, nil, nil, dispatcher, 1, state, owner, nil)
			if !accepted || reason != "" || len(dispatcher.broadcasts) != 0 {
				t.Fatal("admission should reserve the owner without broadcasting")
			}
			logger := &commandTestLogger{}
			got := match.MatchJoin(context.Background(), logger, nil, nil, dispatcher, 1, state,
				[]runtime.Presence{nil, matchTestPresence{"other-user", "session-two"}, owner})
			if got != state || !state.joined || state.playerPresence != owner || state.round != nil {
				t.Fatal("confirmed join should preserve membership and wait for Start even if delivery fails")
			}
			if len(dispatcher.broadcasts) != 1 {
				t.Fatalf("confirmed join should attempt one initial snapshot; got %d", len(dispatcher.broadcasts))
			}
			view := deliveryTestSnapshot(t, dispatcher, 0, owner)
			if view.Phase != PhaseWaiting || !slices.Equal(view.AvailableActions, []int64{1}) {
				t.Errorf("initial snapshot should offer Start: %+v", view)
			}
			wantWarnings := 0
			if failDelivery {
				wantWarnings = 1
			}
			if len(logger.warnings) != wantWarnings {
				t.Errorf("delivery warnings = %v; want %d", logger.warnings, wantWarnings)
			}
		})
	}
}

func TestMatchLoopGetStateIsReadOnly(t *testing.T) {
	for _, phase := range []Phase{PhaseWaiting, PhasePlayerTurn, PhaseDealerTurn, PhaseFinished} {
		t.Run(string(phase), func(t *testing.T) {
			owner := matchTestPresence{"owner", "session-one"}
			state := &BlackjackMatchState{joined: true, ownerUserID: "owner", playerSessionID: "session-one", playerPresence: owner}
			if phase != PhaseWaiting {
				state.round = snapshotTestRound(t)
				state.round.phase = phase
				if phase == PhaseFinished {
					state.round.outcome = OutcomeLoss
				}
			}
			before := *state
			wantView := state.snapshot()
			var oldRound Round
			var oldPlayer, oldDealer, oldCards []Card
			if state.round != nil {
				oldRound = *state.round
				oldPlayer = slices.Clone(state.round.playerHand)
				oldDealer = slices.Clone(state.round.dealerHand)
				oldCards = slices.Clone(state.round.deck.cards)
			}
			match := &BlackjackMatch{}
			dispatcher := &snapshotTestDispatcher{}
			got := match.MatchLoop(context.Background(), &commandTestLogger{}, nil, nil, dispatcher, 10, state,
				[]runtime.MatchData{commandTestMessage{owner, 4, nil}})
			if got != state || *state != before || len(dispatcher.broadcasts) != 1 {
				t.Fatal("GetState should retain state and send exactly one snapshot")
			}
			if view := deliveryTestSnapshot(t, dispatcher, 0, owner); !reflect.DeepEqual(view, wantView) {
				t.Errorf("GetState snapshot = %+v; want %+v", view, wantView)
			}
			if state.round != nil {
				round := state.round
				if round.phase != oldRound.phase || round.outcome != oldRound.outcome || round.deck != oldRound.deck ||
					!slices.Equal(round.playerHand, oldPlayer) || !slices.Equal(round.dealerHand, oldDealer) || !slices.Equal(round.deck.cards, oldCards) {
					t.Error("GetState changed private round data")
				}
			}
		})
	}
}

func TestMatchLoopDoesNotSendUnsolicitedSnapshots(t *testing.T) {
	owner := matchTestPresence{"owner", "session-one"}
	tests := []struct {
		name     string
		message  runtime.MatchData
		idle     bool
		unjoined bool
		noRound  bool
		finished bool
	}{
		{name: "idle tick", idle: true},
		{name: "nil message", message: nil},
		{name: "unknown opcode", message: commandTestMessage{owner, 99, nil}},
		{name: "GetState from other user", message: commandTestMessage{matchTestPresence{"other-user", "session-one"}, 4, nil}},
		{name: "GetState from other session", message: commandTestMessage{matchTestPresence{"owner", "session-two"}, 4, nil}},
		{name: "GetState with empty session", message: commandTestMessage{matchTestPresence{"owner", ""}, 4, nil}},
		{name: "GetState with payload", message: commandTestMessage{owner, 4, []byte(`{}`)}},
		{name: "GetState before joining", message: commandTestMessage{owner, 4, nil}, unjoined: true},
		{name: "Start during active round", message: commandTestMessage{owner, 1, nil}},
		{name: "Hit before Start", message: commandTestMessage{owner, 2, nil}, noRound: true},
		{name: "Stand before Start", message: commandTestMessage{owner, 3, nil}, noRound: true},
		{name: "Hit after completion", message: commandTestMessage{owner, 2, nil}, finished: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &BlackjackMatchState{joined: !tt.unjoined, ownerUserID: "owner", playerSessionID: "session-one", playerPresence: owner}
			if !tt.noRound {
				state.round = snapshotTestRound(t)
				if tt.finished {
					state.round.phase, state.round.outcome = PhaseFinished, OutcomeLoss
				}
			}
			messages := []runtime.MatchData{tt.message}
			if tt.idle {
				messages = nil
			}
			before := *state
			dispatcher := &snapshotTestDispatcher{}
			match := &BlackjackMatch{}
			got := match.MatchLoop(context.Background(), &commandTestLogger{}, nil, nil, dispatcher, 10, state, messages)
			if got != state || *state != before || len(dispatcher.broadcasts) != 0 {
				t.Fatal("idle, ignored, and rejected commands must not send snapshots or replace state")
			}
		})
	}
}

func TestMatchLoopStartDeliversSnapshot(t *testing.T) {
	owner := matchTestPresence{"owner", "session-one"}
	state := &BlackjackMatchState{joined: true, ownerUserID: "owner", playerSessionID: "session-one", playerPresence: owner}
	dispatcher := &snapshotTestDispatcher{}
	match := &BlackjackMatch{}
	got := match.MatchLoop(context.Background(), &commandTestLogger{}, nil, nil, dispatcher, 10, state,
		[]runtime.MatchData{commandTestMessage{owner, 1, nil}})
	if got != state || state.round == nil || len(dispatcher.broadcasts) != 1 {
		t.Fatal("successful Start must create a round and send exactly one snapshot")
	}
	if view := deliveryTestSnapshot(t, dispatcher, 0, owner); !reflect.DeepEqual(view, state.snapshot()) {
		t.Error("Start sent a snapshot that does not match the newly dealt round")
	}
}

func TestMatchLoopSnapshotOrderAndDeliveryRecovery(t *testing.T) {
	for _, failDelivery := range []bool{false, true} {
		name := "ordered successful delivery"
		if failDelivery {
			name = "delivery failure preserves actions and permits GetState recovery"
		}
		t.Run(name, func(t *testing.T) {
			round, drawCards, remaining := newActionTestRound(t, [2]int{10, 6}, [2]int{9, 7}, []int{3, 2})
			wantPlayer := append(slices.Clone(round.playerHand), drawCards[0])
			wantDealer := append(slices.Clone(round.dealerHand), drawCards[1])
			owner := matchTestPresence{"owner", "session-one"}
			state := &BlackjackMatchState{round: round, joined: true, ownerUserID: "owner", playerSessionID: "session-one", playerPresence: owner}
			dispatcher := &snapshotTestDispatcher{}
			if failDelivery {
				dispatcher.err = errors.New("delivery failed")
			}
			logger := &commandTestLogger{}
			match := &BlackjackMatch{}
			got := match.MatchLoop(context.Background(), logger, nil, nil, dispatcher, 10, state,
				[]runtime.MatchData{commandTestMessage{owner, 2, nil}, commandTestMessage{owner, 3, nil}})
			if got != state || len(dispatcher.broadcasts) != 2 {
				t.Fatal("each successful action should attempt one snapshot even when delivery fails")
			}
			assertCompletedAction(t, round, wantPlayer, wantDealer, []Card{remaining}, OutcomeWon)
			first := deliveryTestSnapshot(t, dispatcher, 0, owner)
			second := deliveryTestSnapshot(t, dispatcher, 1, owner)
			if first.Phase != PhasePlayerTurn || first.PlayerTotal != 19 || first.DealerTotal != 9 || first.DealerHiddenCount != 1 || len(first.DealerHand) != 1 {
				t.Errorf("first snapshot should reflect Hit before Stand: %+v", first)
			}
			if second.Phase != PhaseFinished || second.PlayerTotal != 19 || second.DealerTotal != 18 || second.DealerHiddenCount != 0 || second.Outcome != OutcomeWon {
				t.Errorf("second snapshot should reflect the completed dealer turn: %+v", second)
			}
			wantWarnings := 0
			if failDelivery {
				wantWarnings = 2
			}
			if len(logger.warnings) != wantWarnings {
				t.Errorf("delivery warnings = %v; want %d", logger.warnings, wantWarnings)
			}
			dispatcher.err = nil
			got = match.MatchLoop(context.Background(), logger, nil, nil, dispatcher, 11, state, []runtime.MatchData{commandTestMessage{owner, 4, nil}})
			if got != state || len(dispatcher.broadcasts) != 3 {
				t.Fatal("GetState should send the latest state after delivery resumes")
			}
			if recovered := deliveryTestSnapshot(t, dispatcher, 2, owner); !reflect.DeepEqual(recovered, state.snapshot()) {
				t.Error("GetState did not recover the current completed round")
			}
			assertCompletedAction(t, round, wantPlayer, wantDealer, []Card{remaining}, OutcomeWon)
		})
	}
}

func TestMatchLoopActionErrorDoesNotBlockNextCommand(t *testing.T) {
	round := snapshotTestRound(t)
	owner := matchTestPresence{"owner", "session-one"}
	state := &BlackjackMatchState{round: round, joined: true, ownerUserID: "owner", playerSessionID: "session-one", playerPresence: owner}
	dispatcher := &snapshotTestDispatcher{}
	logger := &commandTestLogger{}
	match := &BlackjackMatch{}
	got := match.MatchLoop(context.Background(), logger, nil, nil, dispatcher, 10, state,
		[]runtime.MatchData{commandTestMessage{owner, 1, nil}, commandTestMessage{owner, 4, nil}})
	if got != state || len(dispatcher.broadcasts) != 1 || len(logger.warnings) != 1 {
		t.Fatal("a rejected Start must not prevent a subsequent GetState in the same batch")
	}
	if view := deliveryTestSnapshot(t, dispatcher, 0, owner); !reflect.DeepEqual(view, state.snapshot()) {
		t.Error("GetState after a rejected command returned the wrong snapshot")
	}
}
