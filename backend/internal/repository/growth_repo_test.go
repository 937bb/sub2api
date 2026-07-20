package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrowthLeaderboardRewardAllowed(t *testing.T) {
	tests := []struct {
		name           string
		periodSpend    float64
		totalRecharged float64
		lifetimeReward float64
		reward         float64
		ratio          float64
		want           bool
		wantMaximum    float64
	}{
		{name: "valid paid spend", periodSpend: 20, totalRecharged: 100, lifetimeReward: 5, reward: 10, ratio: 0.5, want: true, wantMaximum: 50},
		{name: "free balance spend", periodSpend: 20, totalRecharged: 0, reward: 5, ratio: 1, want: false, wantMaximum: 0},
		{name: "reward exceeds spend", periodSpend: 4, totalRecharged: 100, reward: 5, ratio: 1, want: false, wantMaximum: 100},
		{name: "lifetime paid cap", periodSpend: 20, totalRecharged: 10, lifetimeReward: 8, reward: 3, ratio: 1, want: false, wantMaximum: 10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allowed, maximum := growthLeaderboardRewardAllowed(
				test.periodSpend, test.totalRecharged, test.lifetimeReward,
				test.reward, test.ratio,
			)
			require.Equal(t, test.want, allowed)
			require.Equal(t, test.wantMaximum, maximum)
		})
	}
}
