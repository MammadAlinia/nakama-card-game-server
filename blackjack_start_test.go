package main

import (
	"crypto/rand"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestStartRoundInitialAndRestart(t *testing.T) {
	for _, restart := range []bool{false, true} {
		name := "first round"
		if restart {
			name = "replace finished round"
		}
		t.Run(name, func(t *testing.T) {
			state := &BlackjackMatchState{
				joined: true, ownerUserID: "owner", playerSessionID: "session-one",
				playerPresence: matchTestPresence{"owner", "session-one"},
			}
			if restart {
				state.round = &Round{deck: NewDeck(), phase: PhaseFinished, outcome: OutcomeLoss}
			}
			before := *state
			if err := state.startRound(); err != nil {
				t.Fatalf("startRound() failed: %v", err)
			}
			round := state.round
			if round == nil || round == before.round {
				t.Fatal("successful Start must assign a fresh round")
			}
			wantState := before
			wantState.round = round
			if *state != wantState {
				t.Error("Start changed player information or joined status")
			}
			if round.deck == nil {
				t.Fatal("new round must retain its deck")
			}
			if len(round.playerHand) != 2 || len(round.dealerHand) != 2 || len(round.deck.cards) != 48 {
				t.Fatalf("new round has player=%d, dealer=%d, deck=%d cards; want 2, 2, 48",
					len(round.playerHand), len(round.dealerHand), len(round.deck.cards))
			}
			// An opening blackjack may finish the round immediately.
			if round.phase != PhasePlayerTurn && round.phase != PhaseFinished {
				t.Errorf("unexpected opening phase %q", round.phase)
			}
			counts := make(map[Card]int)
			for _, cards := range [][]Card{round.playerHand, round.dealerHand, round.deck.cards} {
				for _, card := range cards {
					counts[card]++
				}
			}
			for _, card := range NewDeck().cards {
				if counts[card] != 1 {
					t.Errorf("card %v occurs %d times across hands and deck; want once", card, counts[card])
				}
			}
			if restart && (before.round.phase != PhaseFinished || before.round.outcome != OutcomeLoss || len(before.round.deck.cards) != 52) {
				t.Error("starting again changed the previous round instead of replacing it")
			}
		})
	}
}

func TestStartRoundRejectedPreservesState(t *testing.T) {
	tests := []struct {
		name     string
		joined   bool
		hasRound bool
		phase    Phase
	}{
		{"not joined without round", false, false, ""},
		{"not joined with finished round", false, true, PhaseFinished},
		{"player turn is active", true, true, PhasePlayerTurn},
		{"dealer turn is active", true, true, PhaseDealerTurn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &BlackjackMatchState{joined: tt.joined, ownerUserID: "owner", playerSessionID: "session-one"}
			if tt.hasRound {
				state.round = &Round{deck: NewDeck(), phase: tt.phase, outcome: OutcomeContinue}
			}
			before := *state
			var beforeCards []Card
			if state.round != nil {
				beforeCards = slices.Clone(state.round.deck.cards)
			}
			if err := state.startRound(); err == nil {
				t.Error("Start should reject an unjoined player or an active round")
			}
			if *state != before {
				t.Fatal("rejected Start changed the match state")
			}
			if state.round != nil && (state.round.phase != tt.phase || state.round.outcome != OutcomeContinue || !slices.Equal(state.round.deck.cards, beforeCards)) {
				t.Error("rejected Start changed the previous round")
			}
		})
	}
}

func TestStartRoundShuffleFailurePreservesState(t *testing.T) {
	for _, hasRound := range []bool{false, true} {
		name := "first round"
		if hasRound {
			name = "restart"
		}
		t.Run(name, func(t *testing.T) {
			// Like the deck failure tests, this must not run in parallel.
			originalReader := rand.Reader
			t.Cleanup(func() { rand.Reader = originalReader })
			rand.Reader = strings.NewReader("")
			state := &BlackjackMatchState{joined: true, ownerUserID: "owner", playerSessionID: "session-one"}
			if hasRound {
				state.round = &Round{deck: NewDeck(), phase: PhaseFinished, outcome: OutcomeLoss}
			}
			before := *state
			var beforeCards []Card
			if hasRound {
				beforeCards = slices.Clone(state.round.deck.cards)
			}
			if err := state.startRound(); !errors.Is(err, io.EOF) {
				t.Errorf("Start should propagate the shuffle error; got %v", err)
			}
			if *state != before {
				t.Fatal("shuffle failure changed the match state")
			}
			if hasRound && (state.round.phase != PhaseFinished || state.round.outcome != OutcomeLoss || !slices.Equal(state.round.deck.cards, beforeCards)) {
				t.Error("shuffle failure changed the previous finished round")
			}
		})
	}
}
