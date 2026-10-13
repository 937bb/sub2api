package middleware

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestImageCapacityDistinguishesByteAndRequestLimits(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		maxRequests  int
		weight       int64
	}{
		{"request count", "requests", 1, 8 << 20},
		{"weighted bytes", "bytes", 128, 1024 << 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := &bpsImageAdmissionBudget{limitBytes: 1024 << 20, maxRequests: tc.maxRequests}
			first, _ := pool.acquireObserved(tc.weight)
			require.NotNil(t, first)
			defer first.release()
			blocked, observation := pool.acquireObserved(8 << 20)
			require.Nil(t, blocked)
			require.NotNil(t, observation)
			require.Equal(t, tc.reason, observation.reason)
			require.Equal(t, 1, observation.requests)
			require.Equal(t, tc.weight, observation.reservedBytes)
			require.Equal(t, uint64(1), observation.requestsRejected+observation.bytesRejected)
			blocked, observation = pool.acquireObserved(8 << 20)
			require.Nil(t, blocked)
			require.Nil(t, observation, "rapid failures must not flood logs")
			require.Equal(t, uint64(2), pool.observed.requestsRejected+pool.observed.bytesRejected)
		})
	}
}

func TestImageCapacityHotBudgetIncreasePreservesReservations(t *testing.T) {
	pool := &bpsImageAdmissionBudget{limitBytes: 1024 << 20, maxRequests: 128}
	first, _ := pool.acquireObserved(1024 << 20)
	require.NotNil(t, first)
	defer first.release()
	blocked, _ := pool.acquireObserved(64 << 20)
	require.Nil(t, blocked)
	pool.configure(4096<<20, 128)
	next, _ := pool.acquireObserved(64 << 20)
	require.NotNil(t, next, "hot budget changes must take effect without restarting")
	require.Equal(t, 2, pool.requests)
	require.Equal(t, int64(1088<<20), pool.bytes)
	first.release()
	next.release()
	next.release()
	require.Zero(t, pool.requests)
	require.Zero(t, pool.bytes)
}

func TestImageCapacityPeriodicReportsRetainSuppressedRejectionCounts(t *testing.T) {
	pool := &bpsImageAdmissionBudget{limitBytes: 1024 << 20, maxRequests: 1}
	first, observation := pool.acquireObserved(8 << 20)
	require.NotNil(t, first)
	defer first.release()
	require.NotNil(t, observation)
	for i := 0; i < 5; i++ {
		blocked, _ := pool.acquireObserved(8 << 20)
		require.Nil(t, blocked)
	}
	first.release()
	pool.observed.lastPeriodic = time.Now().Add(-time.Minute)
	next, observation := pool.acquireObserved(8 << 20)
	require.NotNil(t, next)
	defer next.release()
	require.NotNil(t, observation)
	require.Equal(t, uint64(5), observation.requestsRejected)
	require.Equal(t, uint64(2), observation.admitted)
}
