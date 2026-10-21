package wsmux

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetiredIDsCoalesceOutOfOrder(t *testing.T) {
	for _, first := range []uint32{1, 2} {
		var ids retiredIDs
		// Build separate ranges, then bridge them and extend both ends.
		for _, offset := range []uint32{4, 8, 6, 2, 0, 10, 6} {
			require.True(t, ids.add(first+offset))
		}
		require.Len(t, ids.ranges, 1)
		assert.Equal(t, streamIDRange{first: first, last: first + 10}, ids.ranges[0])
		for offset := uint32(0); offset <= 10; offset += 2 {
			assert.True(t, ids.contains(first+offset))
			assert.False(t, ids.contains(first+offset+1))
		}
		assert.False(t, ids.contains(first+12))
	}
}

func TestRetiredIDsMatchClosedIDs(t *testing.T) {
	var ids retiredIDs
	closed := make(map[uint32]bool)
	rng := rand.New(rand.NewPCG(1, 2))
	for _, n := range rng.Perm(500) {
		id := uint32(2*n + 1)
		require.True(t, ids.add(id))
		closed[id] = true
		for candidate := uint32(1); candidate < 1000; candidate += 2 {
			require.Equal(t, closed[candidate], ids.contains(candidate), "ID %d", candidate)
		}
	}
	assert.Len(t, ids.ranges, 1)
}

func TestRetiredIDsLimitPreservesHistory(t *testing.T) {
	var ids retiredIDs
	for i := uint32(0); i < maxRetiredRanges; i++ {
		require.True(t, ids.add(4*i+1))
	}
	assert.False(t, ids.add(4*maxRetiredRanges+1))
	assert.Len(t, ids.ranges, maxRetiredRanges)
	for i := uint32(0); i < maxRetiredRanges; i++ {
		assert.True(t, ids.contains(4*i+1))
	}
	// The cap still permits duplicates and IDs that merge existing ranges.
	require.True(t, ids.add(1))
	require.True(t, ids.add(3))
	assert.Len(t, ids.ranges, maxRetiredRanges-1)
	require.True(t, ids.add(4*maxRetiredRanges+1))
}

func TestRetiredIDsAtIDLimit(t *testing.T) {
	for _, last := range []uint32{math.MaxUint32, math.MaxUint32 - 1} {
		var ids retiredIDs
		require.True(t, ids.add(last))
		require.True(t, ids.add(last-4))
		require.True(t, ids.add(last-2))
		require.Len(t, ids.ranges, 1)
		assert.Equal(t, streamIDRange{first: last - 4, last: last}, ids.ranges[0])
		assert.True(t, ids.contains(last))
		assert.False(t, ids.contains(last-6))
	}
}
