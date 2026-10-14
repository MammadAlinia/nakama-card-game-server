package main

import (
	"fmt"
	"slices"
	"testing"
)

func TestNewRoundOpeningDeal(t *testing.T) {
	deck := &Deck{cards: []Card{
		{rank: 10, suit: "hearts"},
		{rank: 8, suit: "spades"},
		{rank: 7, suit: "clubs"},
		{rank: 1, suit: "diamonds"},
		{rank: 5, suit: "clubs"},
	}}

	round, err := NewRound(deck)
	if err != nil {
		t.Fatalf("NewRound() returned an unexpected error: %v", err)
	}
	if round == nil {
		t.Fatal("NewRound() returned a nil round without an error")
	}

	wantPlayer := []Card{
		{rank: 10, suit: "hearts"},
		{rank: 7, suit: "clubs"},
	}
	wantDealer := []Card{
		{rank: 8, suit: "spades"},
		{rank: 1, suit: "diamonds"},
	}
	if !slices.Equal(round.playerHand, wantPlayer) {
		t.Errorf("player hand = %v; want %v", round.playerHand, wantPlayer)
	}
	if !slices.Equal(round.dealerHand, wantDealer) {
		t.Errorf("dealer hand = %v; want %v", round.dealerHand, wantDealer)
	}
	if value := HandValue(round.playerHand); value != 17 {
		t.Errorf("player total = %d; want 17", value)
	}
	if value := HandValue(round.dealerHand); value != 19 {
		t.Errorf("dealer total = %d; want 19", value)
	}
	if round.deck != deck {
		t.Error("round should retain the supplied deck for later draws")
	}
	wantRemaining := []Card{{rank: 5, suit: "clubs"}}
	if !slices.Equal(deck.cards, wantRemaining) {
		t.Errorf("remaining deck = %v; want %v", deck.cards, wantRemaining)
	}
}

func TestNewRoundInsufficientCards(t *testing.T) {
	for count := 0; count < 4; count++ {
		t.Run(fmt.Sprintf("%d cards", count), func(t *testing.T) {
			deck := NewDeck()
			deck.cards = deck.cards[:count]

			round, err := NewRound(deck)
			if err == nil {
				t.Fatal("NewRound() with fewer than four cards should return an error")
			}
			if round != nil {
				t.Error("a failed opening deal should return a nil round")
			}
		})
	}
}

func TestNewRoundNilDeck(t *testing.T) {
	round, err := NewRound(nil)
	if err == nil {
		t.Fatal("NewRound(nil) should return an error")
	}
	if round != nil {
		t.Error("NewRound(nil) should return a nil round")
	}
}

func TestIsBlackjack(t *testing.T) {
	tests := []struct {
		name  string
		ranks []int
		want  bool
	}{
		{"empty", nil, false},
		{"one ace", []int{1}, false},
		{"ace and ten", []int{1, 10}, true},
		{"ace and jack", []int{1, 11}, true},
		{"ace and queen", []int{1, 12}, true},
		{"ace and king", []int{1, 13}, true},
		{"king and ace", []int{13, 1}, true},
		{"three-card twenty-one", []int{1, 5, 5}, false},
		{"two aces", []int{1, 1}, false},
		{"twenty", []int{10, 13}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards := make([]Card, len(tt.ranks))
			for i, rank := range tt.ranks {
				cards[i] = Card{rank: rank, suit: "clubs"}
			}

			if got := isBlackjack(cards); got != tt.want {
				t.Errorf("IsBlackjack(%v) = %t; want %t", tt.ranks, got, tt.want)
			}
		})
	}
}

func TestNewRoundOpeningOutcomes(t *testing.T) {
	tests := []struct {
		name        string
		playerRanks [2]int
		dealerRanks [2]int
		wantPhase   Phase
		wantOutcome Outcome
	}{
		{"neither blackjack", [2]int{10, 7}, [2]int{8, 1}, PhasePlayerTurn, OutcomeContinue},
		{"player blackjack", [2]int{1, 13}, [2]int{8, 1}, PhaseFinished, OutcomeWon},
		{"dealer blackjack", [2]int{10, 7}, [2]int{1, 12}, PhaseFinished, OutcomeLoss},
		{"both blackjack", [2]int{1, 13}, [2]int{1, 12}, PhaseFinished, OutcomeDraw},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The opening deal alternates player, dealer, player, dealer.
			remainingCard := Card{rank: 5, suit: "spades"}
			deck := &Deck{cards: []Card{
				{rank: tt.playerRanks[0], suit: "clubs"},
				{rank: tt.dealerRanks[0], suit: "hearts"},
				{rank: tt.playerRanks[1], suit: "clubs"},
				{rank: tt.dealerRanks[1], suit: "hearts"},
				remainingCard,
			}}

			round, err := NewRound(deck)
			if err != nil {
				t.Fatalf("NewRound() returned an unexpected error: %v", err)
			}
			if round == nil {
				t.Fatal("NewRound() returned a nil round without an error")
			}
			if round.phase != tt.wantPhase {
				t.Errorf("phase = %q; want %q", round.phase, tt.wantPhase)
			}
			if round.outcome != tt.wantOutcome {
				t.Errorf("outcome = %q; want %q", round.outcome, tt.wantOutcome)
			}
			if !slices.Equal(deck.cards, []Card{remainingCard}) {
				t.Errorf("remaining deck = %v; want only %v", deck.cards, remainingCard)
			}
		})
	}
}

func TestHit(t *testing.T) {
	tests := []struct {
		name        string
		playerRanks [2]int
		drawRank    int
		wantTotal   int
		wantPhase   Phase
		wantOutcome Outcome
	}{
		{"seventeen to nineteen", [2]int{10, 7}, 2, 19, PhasePlayerTurn, OutcomeContinue},
		{"twenty-one automatically finishes against seventeen", [2]int{10, 7}, 4, 21, PhaseFinished, OutcomeWon},
		{"bust finishes with loss", [2]int{10, 7}, 5, 22, PhaseFinished, OutcomeLoss},
		{"ace reduced to one", [2]int{1, 6}, 10, 17, PhasePlayerTurn, OutcomeContinue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drawnCard := Card{rank: tt.drawRank, suit: "diamonds"}
			remainingCard := Card{rank: 3, suit: "spades"}
			deck := &Deck{cards: []Card{
				{rank: tt.playerRanks[0], suit: "clubs"},
				{rank: 9, suit: "hearts"},
				{rank: tt.playerRanks[1], suit: "clubs"},
				{rank: 8, suit: "hearts"},
				drawnCard,
				remainingCard,
			}}
			round, err := NewRound(deck)
			if err != nil {
				t.Fatalf("NewRound() returned an unexpected error: %v", err)
			}
			wantPlayer := append(slices.Clone(round.playerHand), drawnCard)
			wantDealer := slices.Clone(round.dealerHand)

			if err := round.Hit(); err != nil {
				t.Fatalf("Hit() returned an unexpected error: %v", err)
			}
			if !slices.Equal(round.playerHand, wantPlayer) {
				t.Errorf("player hand = %v; want %v", round.playerHand, wantPlayer)
			}
			if total := HandValue(round.playerHand); total != tt.wantTotal {
				t.Errorf("player total = %d; want %d", total, tt.wantTotal)
			}
			if round.phase != tt.wantPhase {
				t.Errorf("phase = %q; want %q", round.phase, tt.wantPhase)
			}
			if round.outcome != tt.wantOutcome {
				t.Errorf("outcome = %q; want %q", round.outcome, tt.wantOutcome)
			}
			if !slices.Equal(round.dealerHand, wantDealer) {
				t.Errorf("dealer hand changed: got %v; want %v", round.dealerHand, wantDealer)
			}
			if round.deck != deck {
				t.Error("Hit() replaced the round's deck")
			}
			if !slices.Equal(deck.cards, []Card{remainingCard}) {
				t.Errorf("remaining deck = %v; want only %v", deck.cards, remainingCard)
			}
		})
	}
}

func TestHitErrorsPreserveState(t *testing.T) {
	tests := []struct {
		name    string
		phase   Phase
		outcome Outcome
		cards   []Card
	}{
		{"dealer turn", PhaseDealerTurn, OutcomeContinue, []Card{{rank: 2, suit: "spades"}}},
		{"finished round", PhaseFinished, OutcomeWon, []Card{{rank: 2, suit: "spades"}}},
		{"empty deck", PhasePlayerTurn, OutcomeContinue, []Card{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			round := &Round{
				deck:       &Deck{cards: slices.Clone(tt.cards)},
				playerHand: []Card{{rank: 10, suit: "clubs"}, {rank: 7, suit: "clubs"}},
				dealerHand: []Card{{rank: 9, suit: "hearts"}, {rank: 8, suit: "hearts"}},
				phase:      tt.phase,
				outcome:    tt.outcome,
			}
			// Copy slices so mutations cannot also change the expected state.
			beforePlayer := slices.Clone(round.playerHand)
			beforeDealer := slices.Clone(round.dealerHand)
			beforeDeck := round.deck
			beforeCards := slices.Clone(round.deck.cards)
			beforePhase, beforeOutcome := round.phase, round.outcome

			if err := round.Hit(); err == nil {
				t.Error("Hit() should return an error")
			}
			if !slices.Equal(round.playerHand, beforePlayer) {
				t.Errorf("player hand changed: got %v; want %v", round.playerHand, beforePlayer)
			}
			if !slices.Equal(round.dealerHand, beforeDealer) {
				t.Errorf("dealer hand changed: got %v; want %v", round.dealerHand, beforeDealer)
			}
			if round.deck != beforeDeck {
				t.Fatal("Hit() replaced the round's deck")
			}
			if !slices.Equal(round.deck.cards, beforeCards) {
				t.Errorf("deck changed: got %v; want %v", round.deck.cards, beforeCards)
			}
			if round.phase != beforePhase {
				t.Errorf("phase changed: got %q; want %q", round.phase, beforePhase)
			}
			if round.outcome != beforeOutcome {
				t.Errorf("outcome changed: got %q; want %q", round.outcome, beforeOutcome)
			}
		})
	}
}

func TestStand(t *testing.T) {
	tests := []struct {
		name        string
		playerRanks [2]int
		dealerRanks [2]int
		drawRanks   []int
		wantDraws   int
		wantTotal   int
		wantOutcome Outcome
	}{
		{"dealer stands on seventeen and player wins", [2]int{10, 9}, [2]int{10, 7}, nil, 0, 17, OutcomeWon},
		{"higher dealer total wins", [2]int{10, 7}, [2]int{10, 9}, nil, 0, 19, OutcomeLoss},
		{"equal totals draw", [2]int{10, 7}, [2]int{10, 7}, nil, 0, 17, OutcomeDraw},
		{"dealer stands on soft seventeen", [2]int{10, 7}, [2]int{1, 6}, []int{10}, 0, 17, OutcomeDraw},
		{"dealer draws to twenty-one", [2]int{10, 9}, [2]int{10, 6}, []int{5}, 1, 21, OutcomeLoss},
		{"dealer bust gives player the win", [2]int{10, 9}, [2]int{10, 6}, []int{10}, 1, 26, OutcomeWon},
		{"dealer draws multiple cards", [2]int{10, 8}, [2]int{2, 3}, []int{4, 8}, 2, 17, OutcomeWon},
		{"dealer recalculates ace value", [2]int{10, 9}, [2]int{1, 5}, []int{10, 2}, 2, 18, OutcomeWon},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			round, drawCards, remaining := newActionTestRound(t, tt.playerRanks, tt.dealerRanks, tt.drawRanks)
			beforePlayer := slices.Clone(round.playerHand)
			wantDealer := append(slices.Clone(round.dealerHand), drawCards[:tt.wantDraws]...)
			wantRemaining := append(slices.Clone(drawCards[tt.wantDraws:]), remaining)

			if err := round.Stand(); err != nil {
				t.Fatalf("Stand() returned an unexpected error: %v", err)
			}
			assertCompletedAction(t, round, beforePlayer, wantDealer, wantRemaining, tt.wantOutcome)
			if total := HandValue(round.dealerHand); total != tt.wantTotal {
				t.Errorf("dealer total = %d; want %d", total, tt.wantTotal)
			}
		})
	}
}

func TestHitTwentyOneResolvesDealer(t *testing.T) {
	tests := []struct {
		name        string
		dealerRanks [2]int
		dealerDraws []int
		wantOutcome Outcome
	}{
		{"dealer draws and player wins", [2]int{10, 6}, []int{2}, OutcomeWon},
		{"dealer reaches twenty-one and draws", [2]int{10, 6}, []int{5}, OutcomeDraw},
		{"dealer busts", [2]int{10, 6}, []int{10}, OutcomeWon},
		{"dealer draws multiple cards", [2]int{2, 3}, []int{4, 8}, OutcomeWon},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The player takes the first extra card; all others are for the dealer.
			drawRanks := append([]int{4}, tt.dealerDraws...)
			round, drawCards, remaining := newActionTestRound(t, [2]int{10, 7}, tt.dealerRanks, drawRanks)
			wantPlayer := append(slices.Clone(round.playerHand), drawCards[0])
			wantDealer := append(slices.Clone(round.dealerHand), drawCards[1:]...)

			if err := round.Hit(); err != nil {
				t.Fatalf("Hit() returned an unexpected error: %v", err)
			}
			assertCompletedAction(t, round, wantPlayer, wantDealer, []Card{remaining}, tt.wantOutcome)
		})
	}
}

func TestStandRejectedPreservesState(t *testing.T) {
	for _, phase := range []Phase{PhaseDealerTurn, PhaseFinished} {
		t.Run(string(phase), func(t *testing.T) {
			round, _, _ := newActionTestRound(t, [2]int{10, 7}, [2]int{10, 6}, []int{5})
			round.phase = phase
			if phase == PhaseFinished {
				round.outcome = OutcomeLoss
			}
			beforePlayer := slices.Clone(round.playerHand)
			beforeDealer := slices.Clone(round.dealerHand)
			beforeDeck := round.deck
			beforeCards := slices.Clone(round.deck.cards)
			beforeOutcome := round.outcome

			if err := round.Stand(); err == nil {
				t.Error("Stand() should reject an action outside the player's turn")
			}
			if !slices.Equal(round.playerHand, beforePlayer) || !slices.Equal(round.dealerHand, beforeDealer) {
				t.Error("rejected Stand() changed a hand")
			}
			if round.deck != beforeDeck || !slices.Equal(round.deck.cards, beforeCards) {
				t.Error("rejected Stand() changed the deck")
			}
			if round.phase != phase || round.outcome != beforeOutcome {
				t.Error("rejected Stand() changed the phase or outcome")
			}
		})
	}
}

func TestDealerDrawError(t *testing.T) {
	tests := []struct {
		name        string
		hit         bool
		dealerRanks [2]int
		drawRanks   []int
	}{
		{"stand with no dealer card available", false, [2]int{10, 6}, nil},
		{"stand consumes one card then runs out", false, [2]int{2, 3}, []int{4}},
		{"hit reaches twenty-one then dealer runs out", true, [2]int{10, 6}, []int{4}},
		{"hit reaches twenty-one and dealer consumes one card then runs out", true, [2]int{2, 3}, []int{4, 6}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			round, drawCards, _ := newActionTestRound(t, [2]int{10, 7}, tt.dealerRanks, tt.drawRanks)
			// Remove the helper's spare card to force exhaustion during dealer play.
			round.deck.cards = round.deck.cards[:len(drawCards)]
			wantPlayer := slices.Clone(round.playerHand)
			wantDealer := slices.Clone(round.dealerHand)
			if tt.hit {
				wantPlayer = append(wantPlayer, drawCards[0])
				wantDealer = append(wantDealer, drawCards[1:]...)
			} else {
				wantDealer = append(wantDealer, drawCards...)
			}

			var err error
			if tt.hit {
				err = round.Hit()
			} else {
				err = round.Stand()
			}
			if err == nil {
				t.Fatal("dealer draw failure should return an error")
			}
			if round.phase != PhaseDealerTurn || round.outcome != OutcomeContinue {
				t.Errorf("failed dealer play has phase %q, outcome %q; want dealer_turn, continue", round.phase, round.outcome)
			}
			if !slices.Equal(round.playerHand, wantPlayer) || !slices.Equal(round.dealerHand, wantDealer) {
				t.Error("failed dealer play should retain cards already drawn")
			}
			if len(round.deck.cards) != 0 {
				t.Error("cards already drawn should remain consumed")
			}
		})
	}
}

func TestHitBustSkipsDealer(t *testing.T) {
	round, drawCards, remaining := newActionTestRound(t, [2]int{10, 7}, [2]int{10, 6}, []int{5, 2})
	wantPlayer := append(slices.Clone(round.playerHand), drawCards[0])
	wantDealer := slices.Clone(round.dealerHand)
	wantRemaining := append(slices.Clone(drawCards[1:]), remaining)

	if err := round.Hit(); err != nil {
		t.Fatalf("a successful Hit() that busts should not return an error: %v", err)
	}
	// The dealer has only sixteen, but must not draw after the player busts.
	assertCompletedAction(t, round, wantPlayer, wantDealer, wantRemaining, OutcomeLoss)
}

func TestRoundHitThenStand(t *testing.T) {
	round, drawCards, remaining := newActionTestRound(t, [2]int{10, 6}, [2]int{9, 7}, []int{3, 2})
	wantPlayer := append(slices.Clone(round.playerHand), drawCards[0])
	wantDealer := append(slices.Clone(round.dealerHand), drawCards[1])

	if err := round.Hit(); err != nil {
		t.Fatalf("Hit() returned an unexpected error: %v", err)
	}
	if round.phase != PhasePlayerTurn || round.outcome != OutcomeContinue {
		t.Fatalf("a Hit() to nineteen should leave the player's turn active; got %q, %q", round.phase, round.outcome)
	}
	if err := round.Stand(); err != nil {
		t.Fatalf("Stand() returned an unexpected error: %v", err)
	}
	// The player has nineteen; the dealer draws to eighteen.
	assertCompletedAction(t, round, wantPlayer, wantDealer, []Card{remaining}, OutcomeWon)

	completedDeck := round.deck
	for _, action := range []struct {
		name string
		run  func() error
	}{
		{"Hit", round.Hit},
		{"Stand", round.Stand},
	} {
		t.Run("reject "+action.name+" after completion", func(t *testing.T) {
			if err := action.run(); err == nil {
				t.Errorf("%s() after completion should return an error", action.name)
			}
			if round.deck != completedDeck {
				t.Fatal("a rejected action replaced the deck")
			}
			assertCompletedAction(t, round, wantPlayer, wantDealer, []Card{remaining}, OutcomeWon)
		})
	}
}

// Use a known opening deal and extra cards, with one spare card to check when drawing stops.
func newActionTestRound(t *testing.T, playerRanks, dealerRanks [2]int, drawRanks []int) (*Round, []Card, Card) {
	t.Helper()
	deck := &Deck{cards: []Card{
		{rank: playerRanks[0], suit: "clubs"},
		{rank: dealerRanks[0], suit: "hearts"},
		{rank: playerRanks[1], suit: "clubs"},
		{rank: dealerRanks[1], suit: "hearts"},
	}}
	drawCards := make([]Card, len(drawRanks))
	for i, rank := range drawRanks {
		drawCards[i] = Card{rank: rank, suit: "diamonds"}
	}
	remaining := Card{rank: 9, suit: "spades"}
	deck.cards = append(deck.cards, drawCards...)
	deck.cards = append(deck.cards, remaining)
	round, err := NewRound(deck)
	if err != nil {
		t.Fatalf("NewRound() returned an unexpected error: %v", err)
	}
	if round.phase != PhasePlayerTurn {
		t.Fatal("test setup must start in the player's turn")
	}
	return round, drawCards, remaining
}

func assertCompletedAction(t *testing.T, round *Round, player, dealer, remaining []Card, outcome Outcome) {
	t.Helper()
	if !slices.Equal(round.playerHand, player) {
		t.Errorf("player hand = %v; want %v", round.playerHand, player)
	}
	if !slices.Equal(round.dealerHand, dealer) {
		t.Errorf("dealer hand = %v; want %v", round.dealerHand, dealer)
	}
	if !slices.Equal(round.deck.cards, remaining) {
		t.Errorf("remaining deck = %v; want %v", round.deck.cards, remaining)
	}
	if round.phase != PhaseFinished {
		t.Errorf("phase = %q; want %q", round.phase, PhaseFinished)
	}
	if round.outcome != outcome {
		t.Errorf("outcome = %q; want %q", round.outcome, outcome)
	}
}
