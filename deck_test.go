package main

import (
	"crypto/rand"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestDeck(t *testing.T) {
	deck := NewDeck()

	if len(deck.cards) != 52 {
		t.Fatalf("NewDeck() has %d cards; want 52", len(deck.cards))
	}

	validSuits := map[string]bool{
		"clubs":    true,
		"hearts":   true,
		"spades":   true,
		"diamonds": true,
	}
	seen := make(map[Card]bool)

	for _, card := range deck.cards {
		if card.rank < 1 || card.rank > 13 {
			t.Errorf("invalid rank: %v", card)
		}
		if !validSuits[card.suit] {
			t.Errorf("invalid suit: %v", card)
		}
		if seen[card] {
			t.Errorf("duplicate card: %v", card)
		}
		seen[card] = true
	}

	expectedCards := []Card{
		{rank: 1, suit: "clubs"},
		{rank: 2, suit: "clubs"},
	}

	for i, expected := range expectedCards {
		card, err := deck.Draw()
		if err != nil {
			t.Fatalf("draw %d returned an unexpected error: %v", i+1, err)
		}
		if card != expected {
			t.Errorf("draw %d returned %v; want %v", i+1, card, expected)
		}

		expectedRemaining := 52 - (i + 1)
		if len(deck.cards) != expectedRemaining {
			t.Errorf("after draw %d, deck has %d cards; want %d",
				i+1, len(deck.cards), expectedRemaining)
		}
	}
}

func TestDrawAllCards(t *testing.T) {
	deck := NewDeck()
	seen := make(map[Card]bool)

	for i := 0; i < 52; i++ {
		card, err := deck.Draw()
		if err != nil {
			t.Fatalf("draw %d returned an unexpected error: %v", i+1, err)
		}
		if seen[card] {
			t.Errorf("card drawn more than once: %v", card)
		}
		seen[card] = true
	}

	if len(deck.cards) != 0 {
		t.Errorf("deck has %d cards after 52 draws; want 0", len(deck.cards))
	}

	if _, err := deck.Draw(); err == nil {
		t.Fatal("draw 53 should return an error")
	}
}

func TestDrawEmptyDeck(t *testing.T) {
	deck := &Deck{}

	_, err := deck.Draw()
	if err == nil {
		t.Fatal("Draw() on an empty deck should return an error")
	}
}

func TestShufflePreservesCards(t *testing.T) {
	partialDeck := NewDeck()
	partialDeck.cards = partialDeck.cards[7:]

	tests := []struct {
		name string
		deck *Deck
	}{
		{"full deck", NewDeck()},
		{"partially drawn deck", partialDeck},
		{"duplicate cards", &Deck{cards: []Card{
			{rank: 1, suit: "clubs"},
			{rank: 1, suit: "clubs"},
			{rank: 13, suit: "hearts"},
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := slices.Clone(tt.deck.cards)
			if err := tt.deck.Shuffle(); err != nil {
				t.Fatalf("Shuffle() returned an unexpected error: %v", err)
			}
			if len(tt.deck.cards) != len(before) {
				t.Fatalf("Shuffle() left %d cards; want %d", len(tt.deck.cards), len(before))
			}

			counts := make(map[Card]int)
			for _, card := range before {
				counts[card]++
			}
			for _, card := range tt.deck.cards {
				counts[card]--
			}
			for card, count := range counts {
				if count != 0 {
					t.Errorf("Shuffle() changed the count of %v by %d", card, -count)
				}
			}
		})
	}
}

func TestShuffleChangesOrder(t *testing.T) {
	// These tests replace the global reader and must not run in parallel.
	originalReader := rand.Reader
	t.Cleanup(func() { rand.Reader = originalReader })
	// Every random index is zero, making the resulting order deterministic.
	rand.Reader = strings.NewReader("\x00\x00\x00")

	deck := &Deck{cards: []Card{
		{rank: 1, suit: "clubs"},
		{rank: 2, suit: "clubs"},
		{rank: 3, suit: "clubs"},
		{rank: 4, suit: "clubs"},
	}}
	want := []Card{
		{rank: 4, suit: "clubs"},
		{rank: 1, suit: "clubs"},
		{rank: 2, suit: "clubs"},
		{rank: 3, suit: "clubs"},
	}

	if err := deck.Shuffle(); err != nil {
		t.Fatalf("Shuffle() returned an unexpected error: %v", err)
	}
	if !slices.Equal(deck.cards, want) {
		t.Errorf("Shuffle() produced %v; want %v", deck.cards, want)
	}
}

func TestShuffleSmallDecks(t *testing.T) {
	originalReader := rand.Reader
	t.Cleanup(func() { rand.Reader = originalReader })
	// An exhausted reader catches attempts to use randomness unnecessarily.
	rand.Reader = strings.NewReader("")

	tests := []struct {
		name  string
		cards []Card
	}{
		{"nil cards", nil},
		{"empty cards", []Card{}},
		{"one card", []Card{{rank: 1, suit: "clubs"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deck := &Deck{cards: slices.Clone(tt.cards)}
			if err := deck.Shuffle(); err != nil {
				t.Fatalf("Shuffle() returned an unexpected error: %v", err)
			}
			if !slices.Equal(deck.cards, tt.cards) {
				t.Errorf("Shuffle() produced %v; want %v", deck.cards, tt.cards)
			}
		})
	}
}

func TestShuffleRandomSourceError(t *testing.T) {
	originalReader := rand.Reader
	t.Cleanup(func() { rand.Reader = originalReader })
	rand.Reader = strings.NewReader("")

	deck := NewDeck()
	before := slices.Clone(deck.cards)
	if err := deck.Shuffle(); !errors.Is(err, io.EOF) {
		t.Fatalf("Shuffle() returned %v; want io.EOF", err)
	}
	if !slices.Equal(deck.cards, before) {
		t.Error("Shuffle() changed the deck before the first random read succeeded")
	}
}
