// Package hashing states the cost of one password hash: the shape the loader fills in and the
// account rules hash with. It sits below both, so that the feature does not name the loader and the
// loader does not name the feature, and so that the two processes which must agree on a stored hash
// cannot each state the cost for themselves.
package hashing

// Cost is what one password hash costs to compute. The values are configuration rather than
// constants, because the ones that ship come from measurements in the target Docker environment.
type Cost struct {
	MemoryKiB   uint32
	Passes      uint32
	Parallelism uint8

	// Concurrent is how many hashes one instance computes at a time. Beyond it a request is refused
	// rather than queued, because a queue in front of a memory-hard function is how the instance is
	// made to run out of memory.
	Concurrent int
}
