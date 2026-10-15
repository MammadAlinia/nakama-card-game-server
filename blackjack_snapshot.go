package main

import (
	"encoding/json"
	"errors"

	"github.com/heroiclabs/nakama-common/runtime"
)

type CardView struct {
	Rank int    `json:"rank"`
	Suit string `json:"suit"`
}

type StateSnapshot struct {
	Phase             Phase      `json:"phase"`
	PlayerHand        []CardView `json:"player_hand"`
	DealerHand        []CardView `json:"dealer_hand"`
	PlayerTotal       int        `json:"player_total"`
	DealerTotal       int        `json:"dealer_total"`
	DealerHiddenCount int        `json:"dealer_hidden_count"`
	Outcome           Outcome    `json:"outcome"`
	AvailableActions  []int64    `json:"available_actions"`
}

func (s *BlackjackMatchState) snapshot() StateSnapshot {
	snapshot := StateSnapshot{
		Phase:            PhaseWaiting,
		PlayerHand:       make([]CardView, 0),
		DealerHand:       make([]CardView, 0),
		Outcome:          OutcomeContinue,
		AvailableActions: make([]int64, 0),
	}
	if s.round == nil {
		if s.joined {
			snapshot.AvailableActions = []int64{opStart}
		}
		return snapshot
	}

	round := s.round
	snapshot.Phase = round.phase
	snapshot.Outcome = round.outcome
	snapshot.PlayerHand = cardViews(round.playerHand)
	snapshot.PlayerTotal = HandValue(round.playerHand)

	visibleDealerCards := round.dealerHand
	if round.phase != PhaseFinished && len(visibleDealerCards) > 0 {
		snapshot.DealerHiddenCount = len(visibleDealerCards) - 1
		visibleDealerCards = visibleDealerCards[:1]
	}
	snapshot.DealerHand = cardViews(visibleDealerCards)
	snapshot.DealerTotal = HandValue(visibleDealerCards)

	if s.joined {
		switch round.phase {
		case PhasePlayerTurn:
			snapshot.AvailableActions = []int64{opHit, opStand}
		case PhaseFinished:
			snapshot.AvailableActions = []int64{opStart}
		}
	}

	return snapshot
}

func cardViews(cards []Card) []CardView {
	view := make([]CardView, len(cards))
	for i, card := range cards {
		view[i] = CardView{
			Rank: card.rank,
			Suit: card.suit,
		}
	}
	return view
}
func (s *BlackjackMatchState) sendSnapshot(
	dispatcher runtime.MatchDispatcher,
) error {
	if !s.joined || s.playerPresence == nil {

		return errors.New("presence not joined or found in match state")
	}
	snapshot := s.snapshot()
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return dispatcher.BroadcastMessage(
		opState,
		data,
		[]runtime.Presence{s.playerPresence},
		nil,
		true,
	)
}
