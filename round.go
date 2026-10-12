package main

import "errors"

type Round struct {
	deck       *Deck
	playerHand []Card
	dealerHand []Card
}

func NewRound(deck *Deck) (*Round, error) {
	if deck == nil {
		return nil, errors.New("deck is required")
	}

	round := &Round{deck: deck}
	for i := 0; i < 2; i++ {
		playerCard, err := deck.Draw()
		if err != nil {
			return nil, err
		}
		round.playerHand = append(round.playerHand, playerCard)

		dealerCard, err := deck.Draw()
		if err != nil {
			return nil, err
		}
		round.dealerHand = append(round.dealerHand, dealerCard)
	}

	return round, nil
}
