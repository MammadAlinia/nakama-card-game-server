package main

import "errors"

type Round struct {
	deck       *Deck
	playerHand []Card
	dealerHand []Card
	phase      Phase
	outcome    Outcome
}

type Phase string

const (
	PhasePlayerTurn Phase = "player_turn"
	PhaseDealerTurn Phase = "dealer_turn"
	PhaseFinished   Phase = "finished"
)

type Outcome string

const (
	OutcomeWon      Outcome = "won"
	OutcomeDraw     Outcome = "draw"
	OutcomeLoss     Outcome = "loss"
	OutcomeContinue Outcome = "continue"
)

func NewRound(deck *Deck) (*Round, error) {
	if deck == nil {
		return nil, errors.New("deck is required")
	}
	round := &Round{
		deck:       deck,
		playerHand: []Card{},
		dealerHand: []Card{},
	}
	for range 2 {

		card, err := deck.Draw()

		if err != nil {

			return nil, err
		}
		round.playerHand = append(round.playerHand, card)
		card, err = deck.Draw()

		if err != nil {

			return nil, err
		}

		round.dealerHand = append(round.dealerHand, card)

	}

	playerBlackjack := isBlackjack(round.playerHand)
	dealerBlackjack := isBlackjack(round.dealerHand)

	round.phase = PhaseFinished

	switch {
	case playerBlackjack && dealerBlackjack:
		round.outcome = OutcomeDraw
	case playerBlackjack:
		round.outcome = OutcomeWon
	case dealerBlackjack:
		round.outcome = OutcomeLoss
	default:
		round.phase = PhasePlayerTurn
		round.outcome = OutcomeContinue
	}
	return round, nil
}
func (r *Round) Hit() error {
	if r.phase != PhasePlayerTurn {
		return errors.New("not players turn")
	}
	card, err := r.deck.Draw()
	if err != nil {
		return err
	}
	r.playerHand = append(r.playerHand, card)
	playerTotal := HandValue(r.playerHand)

	if playerTotal == 21 {
		r.outcome = OutcomeContinue
		r.phase = PhaseDealerTurn
		err = r.playDealerHand()
		if err != nil {
			return err
		}
	} else if playerTotal > 21 {
		r.outcome = OutcomeLoss
		r.phase = PhaseFinished
	} else if playerTotal < 21 {
		r.outcome = OutcomeContinue
		r.phase = PhasePlayerTurn
	}

	return nil

}
func isBlackjack(cards []Card) bool {

	if len(cards) != 2 {
		return false
	}
	return HandValue(cards) == 21
}
func (r *Round) Stand() error {

	if r.phase != PhasePlayerTurn {
		return errors.New("not players turn")
	}
	r.phase = PhaseDealerTurn
	r.outcome = OutcomeContinue

	err := r.playDealerHand()
	if err != nil {
		return err
	}

	return nil
}

func (r *Round) playDealerHand() error {
	if r.phase != PhaseDealerTurn {
		return errors.New("not dealers turn")
	}
	for HandValue(r.dealerHand) < 17 {

		card, err := r.deck.Draw()
		if err != nil {
			return err
		}
		r.dealerHand = append(r.dealerHand, card)

	}
	dealersHand := HandValue(r.dealerHand)
	playersHand := HandValue(r.playerHand)
	switch {
	case dealersHand > 21:
		r.outcome = OutcomeWon
	case playersHand > dealersHand:
		r.outcome = OutcomeWon
	case playersHand == dealersHand:
		r.outcome = OutcomeDraw
	default:
		r.outcome = OutcomeLoss
	}
	r.phase = PhaseFinished
	return nil
}
