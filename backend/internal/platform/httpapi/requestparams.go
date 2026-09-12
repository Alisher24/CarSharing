package httpapi

// Names the contract gives the parameters whose failure carries a code of its own rather than the
// generic validation failure. Every other parameter location and name is read from the contract.
const (
	cursorParameter         = "cursor"
	parameterLocationQuery  = "query"
	parameterLocationHeader = "header"
)

// The locations a violation addresses, which are the parameter locations the contract uses.
const (
	locationBody   = "body"
	locationQuery  = parameterLocationQuery
	locationPath   = "path"
	locationHeader = parameterLocationHeader
)
