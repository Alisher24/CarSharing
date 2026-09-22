package httpapi

// The names the markup declares: one layout every page is framed in, one template per page, and the
// refusal a page states when it has nothing to show.
const (
	inboxLayoutTemplate  = "inboxLayout"
	inboxLettersTemplate = "inboxLetters"
	inboxLetterTemplate  = "inboxLetter"
	inboxRefusalTemplate = "inboxRefusal"

	// inboxStylesheet is the one stylesheet of the pages. The policy they are answered with admits no
	// source outside the page itself, so the layout states it inline rather than fetching it.
	inboxStylesheet = "templates/inbox.css"
)
