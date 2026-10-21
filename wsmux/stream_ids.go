package wsmux

import "sort"

// maxRetiredRanges bounds closed-stream history even when a peer closes IDs
// in a sparse order. Adjacent IDs of the same parity share one range.
const maxRetiredRanges = 1024

type streamIDRange struct {
	first uint32
	last  uint32
}

// retiredIDs stores ranges of closed peer IDs, all with the same parity.
// Sequential stream churn uses one range rather than an entry per stream.
type retiredIDs struct {
	ranges []streamIDRange
}

func (s *retiredIDs) contains(id uint32) bool {
	i := sort.Search(len(s.ranges), func(i int) bool {
		return s.ranges[i].last >= id
	})
	return i < len(s.ranges) && s.ranges[i].first <= id && s.ranges[i].first%2 == id%2
}

// add records a closed peer ID. It returns false if a new disjoint range
// would exceed the history limit. Existing ranges are never evicted, since
// forgetting an ID would let a late DATA frame reopen its stream.
func (s *retiredIDs) add(id uint32) bool {
	i := sort.Search(len(s.ranges), func(i int) bool {
		return uint64(s.ranges[i].last)+2 >= uint64(id)
	})
	if i == len(s.ranges) || uint64(id)+2 < uint64(s.ranges[i].first) {
		if len(s.ranges) == maxRetiredRanges {
			return false
		}
		s.ranges = append(s.ranges, streamIDRange{})
		copy(s.ranges[i+1:], s.ranges[i:])
		s.ranges[i] = streamIDRange{first: id, last: id}
		return true
	}

	r := &s.ranges[i]
	if id < r.first {
		r.first = id
	}
	if id > r.last {
		r.last = id
	}
	if i+1 < len(s.ranges) && uint64(r.last)+2 >= uint64(s.ranges[i+1].first) {
		r.last = s.ranges[i+1].last
		copy(s.ranges[i+1:], s.ranges[i+2:])
		s.ranges = s.ranges[:len(s.ranges)-1]
	}
	return true
}
