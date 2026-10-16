package main

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/heroiclabs/nakama-common/runtime"
)

func TestSendErrorDelivery(t *testing.T) {
	owner := matchTestPresence{"owner", "session-one"}
	state := &BlackjackMatchState{
		round: snapshotTestRound(t), joined: true, ownerUserID: "owner", playerSessionID: "session-one", playerPresence: owner,
	}
	dispatcher := &snapshotTestDispatcher{}
	if err := state.sendError(dispatcher, 2, "round has not started"); err != nil {
		t.Fatalf("sendError() failed: %v", err)
	}
	if len(dispatcher.broadcasts) != 1 {
		t.Fatalf("expected one error reply; got %d broadcasts", len(dispatcher.broadcasts))
	}
	packet := dispatcher.broadcasts[0]
	if packet.opCode != 101 || !packet.reliable || packet.sender != nil || len(packet.targets) != 1 || packet.targets[0] != owner {
		t.Fatalf("error reply must be reliable opcode 101 sent only to the stored player: %+v", packet)
	}
	var reply CommandErrorResponse
	if err := json.Unmarshal(packet.data, &reply); err != nil {
		t.Fatalf("error reply is not valid JSON: %v", err)
	}
	if reply.Command != 2 || reply.Message != "round has not started" {
		t.Errorf("error reply = %+v; want command 2 and the supplied message", reply)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(packet.data, &fields); err != nil {
		t.Fatalf("could not inspect error reply fields: %v", err)
	}
	if len(fields) != 2 || fields["command"] == nil || fields["message"] == nil {
		t.Errorf("error reply must contain only command and message, without private game state: %s", packet.data)
	}
}

func TestSendErrorRequiresConnectedPlayer(t *testing.T) {
	tests := []struct {
		name     string
		joined   bool
		presence runtime.Presence
	}{
		{"not joined", false, matchTestPresence{"owner", "session-one"}},
		{"missing presence", true, nil},
		{"neither joined nor present", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &BlackjackMatchState{joined: tt.joined, playerPresence: tt.presence}
			before := *state
			dispatcher := &snapshotTestDispatcher{}
			if err := state.sendError(dispatcher, 2, "round has not started"); err == nil {
				t.Error("sending an error without a connected player should fail")
			}
			if len(dispatcher.broadcasts) != 0 || *state != before {
				t.Error("rejected delivery must not broadcast or change match state")
			}
		})
	}
}

func TestSendErrorPreservesState(t *testing.T) {
	for _, failDelivery := range []bool{false, true} {
		name := "successful delivery"
		if failDelivery {
			name = "failed delivery"
		}
		t.Run(name, func(t *testing.T) {
			round := snapshotTestRound(t)
			state := &BlackjackMatchState{
				round: round, joined: true, ownerUserID: "owner", playerSessionID: "session-one",
				playerPresence: matchTestPresence{"owner", "session-one"},
			}
			beforeState := *state
			beforeRound := *round
			beforePlayer := slices.Clone(round.playerHand)
			beforeDealer := slices.Clone(round.dealerHand)
			beforeCards := slices.Clone(round.deck.cards)
			dispatcher := &snapshotTestDispatcher{}
			if failDelivery {
				dispatcher.err = errors.New("broadcast failed")
			}
			if err := state.sendError(dispatcher, 1, "round already active"); !errors.Is(err, dispatcher.err) {
				t.Errorf("sendError() should return the broadcast result; got %v, want %v", err, dispatcher.err)
			}
			if len(dispatcher.broadcasts) != 1 {
				t.Errorf("error delivery should be attempted once; got %d attempts", len(dispatcher.broadcasts))
			}
			if *state != beforeState || round.phase != beforeRound.phase || round.outcome != beforeRound.outcome || round.deck != beforeRound.deck ||
				!slices.Equal(round.playerHand, beforePlayer) || !slices.Equal(round.dealerHand, beforeDealer) || !slices.Equal(round.deck.cards, beforeCards) {
				t.Error("sending an error changed player or game state")
			}
		})
	}
}
