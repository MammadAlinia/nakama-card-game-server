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
