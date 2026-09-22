package cursor

import "fmt"

// Descending builds a keyset predicate, ordering and limit from trusted SQL column names.
// firstParameter names the timestamp argument, followed by the UUID and the fetch limit.
func Descending(momentColumn, identifierColumn string, firstParameter int) string {
	return fmt.Sprintf(`(%s, %s) <
        (COALESCE($%d::timestamptz, 'infinity'::timestamptz),
         COALESCE($%d::uuid, '00000000-0000-0000-0000-000000000000'::uuid))
ORDER BY %s DESC, %s DESC
LIMIT $%d`, momentColumn, identifierColumn, firstParameter, firstParameter+1,
		momentColumn, identifierColumn, firstParameter+2)
}

func (position *Position) MomentArgument() any {
	if position == nil {
		return nil
	}
	return position.Moment
}

func (position *Position) IdentifierArgument() any {
	if position == nil {
		return nil
	}
	return position.ID
}
