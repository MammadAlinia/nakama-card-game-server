package main

import "testing"

func TestHandValue(t *testing.T) {
	tests := []struct {
		name  string
		ranks []int
		want  int
	}{
		{"empty", nil, 0},
		{"soft seventeen", []int{1, 6}, 17},
		{"ace reduced", []int{1, 6, 10}, 17},
		{"same cards reordered", []int{10, 6, 1}, 17},
		{"two aces", []int{1, 1, 9}, 21},
		{"both aces reduced", []int{1, 1, 13, 9}, 21},
		{"bust", []int{13, 12, 2}, 22},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards := make([]Card, len(tt.ranks))
			for i, rank := range tt.ranks {
				cards[i] = Card{rank: rank, suit: "clubs"}
			}

			got := HandValue(cards)
			if got != tt.want {
				t.Errorf("HandValue(%v) = %d; want %d",
					tt.ranks, got, tt.want)
			}
		})
	}
}
