package main

type Card struct {
	rank int
	suit string
}

func HandValue(cards []Card) int {

	value := 0
	aces := 0
	for _, card := range cards {
		switch card.rank {
		case 1:
			value += 11
			aces += 1
		case 11, 12, 13:
			value += 10
		default:
			value += card.rank
		}
		for value > 21 && aces > 0 {
			value -= 10
			aces--
		}
	}
	return value
}
