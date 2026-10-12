package main

import (
	"crypto/rand"
	"errors"
	"math/big"
)

type Deck struct {
	cards []Card
}

func NewDeck() *Deck {
	deck := new(Deck)
	deck.cards = make([]Card, 52)
	suits := []string{"clubs", "hearts", "spades", "diamonds"}
	for s := range len(suits) {
		for i := range 13 {
			index := s*13 + i
			deck.cards[index] = Card{
				rank: i + 1,
				suit: suits[s],
			}

		}
	}
	return deck
}
func (d *Deck) Shuffle() error {
	for i := 1; i < len(d.cards); i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return err
		}
		j := int(n.Int64())
		d.cards[i], d.cards[j] = d.cards[j], d.cards[i]

	}

	return nil
}
func (d *Deck) Draw() (Card, error) {
	if len(d.cards) == 0 {
		return Card{}, errors.New("no cards left")
	}
	card := d.cards[0]
	d.cards = d.cards[1:]
	return card, nil
}
