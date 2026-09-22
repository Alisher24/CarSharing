package cursor

const PageSize = 20

// Cut publishes at most limit items and derives the next position only when another item exists.
func Cut[T any](items []T, limit int, position func(T) Position) ([]T, *Position) {
	if len(items) <= limit {
		return items, nil
	}
	published := items[:limit]
	next := position(published[len(published)-1])
	return published, &next
}
