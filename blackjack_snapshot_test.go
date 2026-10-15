package main

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/heroiclabs/nakama-common/runtime"
)

func snapshotTestRound(t *testing.T) *Round {
	t.Helper()
	round, err := NewRound(&Deck{cards: []Card{
		{rank: 10, suit: "hearts"},
		{rank: 8, suit: "spades"},
		{rank: 7, suit: "clubs"},
		{rank: 1, suit: "diamonds"},
		{rank: 5, suit: "clubs"},
	}})
	if err != nil {
		t.Fatalf("round setup failed: %v", err)
	}
	return round
}

func TestSnapshotPhasesAndActions(t *testing.T) {
	tests := []struct {
		name    string
		joined  bool
		phase   Phase
		actions []int64
	}{
		{"waiting without player", false, PhaseWaiting, []int64{}},
		{"waiting with player", true, PhaseWaiting, []int64{1}},
		{"player turn without player", false, PhasePlayerTurn, []int64{}},
		{"player turn with player", true, PhasePlayerTurn, []int64{2, 3}},
		{"dealer turn without player", false, PhaseDealerTurn, []int64{}},
		{"dealer turn with player", true, PhaseDealerTurn, []int64{}},
		{"finished without player", false, PhaseFinished, []int64{}},
		{"finished with player", true, PhaseFinished, []int64{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &BlackjackMatchState{joined: tt.joined}
			wantPlayer := []CardView{}
			wantDealer := []CardView{}
			wantPlayerTotal, wantDealerTotal, wantHidden := 0, 0, 0
			wantOutcome := OutcomeContinue
			if tt.phase != PhaseWaiting {
				state.round = snapshotTestRound(t)
				state.round.phase = tt.phase
				wantPlayer = []CardView{{Rank: 10, Suit: "hearts"}, {Rank: 7, Suit: "clubs"}}
				wantDealer = []CardView{{Rank: 8, Suit: "spades"}}
				wantPlayerTotal, wantDealerTotal, wantHidden = 17, 8, 1
				if tt.phase == PhaseFinished {
					state.round.outcome = OutcomeLoss
					wantOutcome = OutcomeLoss
					wantDealer = append(wantDealer, CardView{Rank: 1, Suit: "diamonds"})
					wantDealerTotal, wantHidden = 19, 0
				}
			}
			got := state.snapshot()
			if got.Phase != tt.phase || got.Outcome != wantOutcome {
				t.Errorf("phase=%q, outcome=%q; want %q, %q", got.Phase, got.Outcome, tt.phase, wantOutcome)
			}
			if !slices.Equal(got.PlayerHand, wantPlayer) || !slices.Equal(got.DealerHand, wantDealer) {
				t.Errorf("visible hands = %v, %v; want %v, %v", got.PlayerHand, got.DealerHand, wantPlayer, wantDealer)
			}
			if got.PlayerTotal != wantPlayerTotal || got.DealerTotal != wantDealerTotal || got.DealerHiddenCount != wantHidden {
				t.Errorf("player total=%d, dealer total=%d, hidden=%d; want %d, %d, %d", got.PlayerTotal, got.DealerTotal, got.DealerHiddenCount, wantPlayerTotal, wantDealerTotal, wantHidden)
			}
			if !slices.Equal(got.AvailableActions, tt.actions) {
				t.Errorf("actions=%v; want %v", got.AvailableActions, tt.actions)
			}
			if got.PlayerHand == nil || got.DealerHand == nil || got.AvailableActions == nil {
				t.Error("hand and action slices must be non-nil so empty arrays serialize as []")
			}
		})
	}
}

func TestSnapshotJSONPrivacy(t *testing.T) {
	state := &BlackjackMatchState{
		joined: true, round: snapshotTestRound(t), ownerUserID: "private-owner", playerSessionID: "private-session",
	}
	data, err := json.Marshal(state.snapshot())
	if err != nil {
		t.Fatalf("snapshot serialization failed: %v", err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("invalid snapshot JSON: %v", err)
	}
	allowed := map[string]bool{
		"phase": true, "player_hand": true, "dealer_hand": true,
		"player_total": true, "dealer_total": true, "dealer_hidden_count": true,
		"outcome": true, "available_actions": true,
	}
	for key := range body {
		if !allowed[key] {
			t.Errorf("unexpected field in public snapshot: %q", key)
		}
	}
	for key := range allowed {
		if _, ok := body[key]; !ok {
			t.Errorf("snapshot omitted public field %q", key)
		}
	}
	var visible StateSnapshot
	if err := json.Unmarshal(data, &visible); err != nil {
		t.Fatalf("could not decode snapshot: %v", err)
	}
	if !slices.Equal(visible.DealerHand, []CardView{{Rank: 8, Suit: "spades"}}) || visible.DealerTotal != 8 || visible.DealerHiddenCount != 1 {
		t.Errorf("serialized snapshot exposed hidden dealer information: %s", data)
	}
	for _, secret := range []string{"diamonds", "private-owner", "private-session"} {
		if strings.Contains(string(data), secret) {
			t.Errorf("snapshot exposed %q: %s", secret, data)
		}
	}

	// Round completion reveals the previously hidden card and its full total.
	state.round.phase, state.round.outcome = PhaseFinished, OutcomeLoss
	data, err = json.Marshal(state.snapshot())
	if err != nil {
		t.Fatalf("finished snapshot serialization failed: %v", err)
	}
	if err := json.Unmarshal(data, &visible); err != nil {
		t.Fatalf("could not decode finished snapshot: %v", err)
	}
	if !slices.Equal(visible.DealerHand, []CardView{{Rank: 8, Suit: "spades"}, {Rank: 1, Suit: "diamonds"}}) || visible.DealerTotal != 19 || visible.DealerHiddenCount != 0 {
		t.Errorf("finished snapshot did not reveal the complete dealer hand: %s", data)
	}
}

func TestSnapshotDoesNotMutateOrShareHands(t *testing.T) {
	round := snapshotTestRound(t)
	state := &BlackjackMatchState{joined: true, round: round, ownerUserID: "owner", playerSessionID: "session"}
	beforeState := *state
	beforeRound := *round
	beforePlayer := slices.Clone(round.playerHand)
	beforeDealer := slices.Clone(round.dealerHand)
	beforeCards := slices.Clone(round.deck.cards)
	view := state.snapshot()
	view.PlayerHand[0].Rank = 2
	view.DealerHand[0].Suit = "hearts"
	if *state != beforeState || round.phase != beforeRound.phase || round.outcome != beforeRound.outcome || round.deck != beforeRound.deck ||
		!slices.Equal(round.playerHand, beforePlayer) || !slices.Equal(round.dealerHand, beforeDealer) || !slices.Equal(round.deck.cards, beforeCards) {
		t.Error("building or changing a snapshot mutated server state")
	}
}

func TestSnapshotCountsAllHiddenDealerCards(t *testing.T) {
	round, err := NewRound(&Deck{cards: []Card{
		{rank: 10, suit: "clubs"}, {rank: 2, suit: "hearts"},
		{rank: 7, suit: "clubs"}, {rank: 3, suit: "hearts"},
		{rank: 4, suit: "diamonds"},
	}})
	if err != nil {
		t.Fatalf("round setup failed: %v", err)
	}
	if err := round.Stand(); err == nil {
		t.Fatal("short deck should run out after the dealer's first draw")
	}
	view := (&BlackjackMatchState{joined: true, round: round}).snapshot()
	if view.Phase != PhaseDealerTurn || view.DealerHiddenCount != 2 || view.DealerTotal != 2 || !slices.Equal(view.DealerHand, []CardView{{Rank: 2, Suit: "hearts"}}) || len(view.AvailableActions) != 0 {
		t.Errorf("unfinished dealer play must hide every card after the first: %+v", view)
	}
}

func TestCardViewsCopiesCards(t *testing.T) {
	if got := cardViews(nil); got == nil || len(got) != 0 {
		t.Errorf("nil cards should produce an empty non-nil view slice; got %v", got)
	}
	cards := []Card{{rank: 1, suit: "clubs"}, {rank: 13, suit: "hearts"}}
	got := cardViews(cards)
	want := []CardView{{Rank: 1, Suit: "clubs"}, {Rank: 13, Suit: "hearts"}}
	if !slices.Equal(got, want) {
		t.Fatalf("cardViews()=%v; want %v without extra zero-value cards", got, want)
	}
	got[0].Rank = 5
	if cards[0].rank != 1 {
		t.Error("changing a card view changed the source card")
	}
}

type snapshotTestBroadcast struct {
	opCode   int64
	data     []byte
	targets  []runtime.Presence
	sender   runtime.Presence
	reliable bool
}

type snapshotTestDispatcher struct {
	runtime.MatchDispatcher
	broadcasts []snapshotTestBroadcast
	err        error
}

func (d *snapshotTestDispatcher) BroadcastMessage(opCode int64, data []byte, targets []runtime.Presence, sender runtime.Presence, reliable bool) error {
	d.broadcasts = append(d.broadcasts, snapshotTestBroadcast{
		opCode: opCode, data: slices.Clone(data), targets: slices.Clone(targets), sender: sender, reliable: reliable,
	})
	return d.err
}

func TestSendSnapshotDelivery(t *testing.T) {
	for _, phase := range []Phase{PhaseWaiting, PhasePlayerTurn, PhaseFinished} {
		t.Run(string(phase), func(t *testing.T) {
			presence := matchTestPresence{"private-owner", "private-session"}
			state := &BlackjackMatchState{
				joined: true, ownerUserID: "private-owner", playerSessionID: "private-session", playerPresence: presence,
			}
			if phase != PhaseWaiting {
				state.round = snapshotTestRound(t)
				state.round.phase = phase
				if phase == PhaseFinished {
					state.round.outcome = OutcomeLoss
				}
			}
			dispatcher := &snapshotTestDispatcher{}
			if err := state.sendSnapshot(dispatcher); err != nil {
				t.Fatalf("sendSnapshot() failed: %v", err)
			}
			if len(dispatcher.broadcasts) != 1 {
				t.Fatalf("expected one snapshot; got %d broadcasts", len(dispatcher.broadcasts))
			}
			message := dispatcher.broadcasts[0]
			if message.opCode != 100 || !message.reliable || message.sender != nil {
				t.Errorf("snapshot should use opcode 100, reliable delivery, and nil sender; got %+v", message)
			}
			if len(message.targets) != 1 || message.targets[0] != presence {
				t.Fatal("snapshot must target exactly the stored player presence")
			}
			var view StateSnapshot
			if err := json.Unmarshal(message.data, &view); err != nil {
				t.Fatalf("broadcast payload is not snapshot JSON: %v", err)
			}
			if view.Phase != phase {
				t.Errorf("broadcast phase = %q; want %q", view.Phase, phase)
			}
			switch phase {
			case PhaseWaiting:
				if len(view.PlayerHand) != 0 || len(view.DealerHand) != 0 || !slices.Equal(view.AvailableActions, []int64{1}) {
					t.Errorf("waiting snapshot should have empty hands and offer Start: %+v", view)
				}
			case PhasePlayerTurn:
				if !slices.Equal(view.PlayerHand, []CardView{{10, "hearts"}, {7, "clubs"}}) || view.PlayerTotal != 17 ||
					!slices.Equal(view.DealerHand, []CardView{{8, "spades"}}) || view.DealerTotal != 8 || view.DealerHiddenCount != 1 ||
					!slices.Equal(view.AvailableActions, []int64{2, 3}) {
					t.Errorf("broadcast exposed incorrect or hidden hand information: %+v", view)
				}
				if strings.Contains(string(message.data), "diamonds") {
					t.Error("broadcast exposed the dealer's hidden card")
				}
			case PhaseFinished:
				if !slices.Equal(view.DealerHand, []CardView{{8, "spades"}, {1, "diamonds"}}) || view.DealerTotal != 19 || view.DealerHiddenCount != 0 ||
					view.Outcome != OutcomeLoss || !slices.Equal(view.AvailableActions, []int64{1}) {
					t.Errorf("finished broadcast should reveal the dealer and offer Start: %+v", view)
				}
			}
			for _, secret := range []string{"private-owner", "private-session", `"deck"`} {
				if strings.Contains(string(message.data), secret) {
					t.Errorf("broadcast payload exposed private field or identity %q", secret)
				}
			}
		})
	}
}

func TestSendSnapshotRequiresConnectedPlayer(t *testing.T) {
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
			if err := state.sendSnapshot(dispatcher); err == nil {
				t.Error("sending without a connected player should return an error")
			}
			if len(dispatcher.broadcasts) != 0 || *state != before {
				t.Error("rejected delivery should neither broadcast nor change state")
			}
		})
	}
}

func TestSendSnapshotBroadcastFailurePreservesState(t *testing.T) {
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
	broadcastError := errors.New("broadcast failed")
	dispatcher := &snapshotTestDispatcher{err: broadcastError}
	if err := state.sendSnapshot(dispatcher); !errors.Is(err, broadcastError) {
		t.Errorf("sendSnapshot() should propagate the broadcast error; got %v", err)
	}
	if len(dispatcher.broadcasts) != 1 {
		t.Errorf("delivery should be attempted once; got %d attempts", len(dispatcher.broadcasts))
	}
	if *state != beforeState || round.phase != beforeRound.phase || round.outcome != beforeRound.outcome || round.deck != beforeRound.deck ||
		!slices.Equal(round.playerHand, beforePlayer) || !slices.Equal(round.dealerHand, beforeDealer) || !slices.Equal(round.deck.cards, beforeCards) {
		t.Error("broadcast failure changed game or player state")
	}
}
