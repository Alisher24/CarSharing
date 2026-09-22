package httpapi

// pageLabels is the vocabulary of the pages: the words the markup states, taken from the one
// declaration of them in Go rather than spelled again in a template. A page cannot come to display a
// label the code does not know about, and a link and the page it opens name the same thing.
type pageLabels struct {
	Moment      string
	Recipient   string
	Subject     string
	Identifier  string
	DeliveryKey string
	Text        string

	Next       string
	Refresh    string
	BackToList string
	JSON       string

	NoLetters     string
	NoLettersHint string
}

// inboxPageLabels is what every page of the inbox states.
func inboxPageLabels() pageLabels {
	return pageLabels{
		Moment:      inboxMomentLabel,
		Recipient:   inboxRecipientLabel,
		Subject:     inboxSubjectLabel,
		Identifier:  inboxIdentifierLabel,
		DeliveryKey: inboxDeliveryKeyLabel,
		Text:        inboxTextLabel,

		Next:       inboxNextPageLink,
		Refresh:    inboxRefreshLink,
		BackToList: inboxBackToList,
		JSON:       inboxJSONLink,

		NoLetters:     inboxNoLetters,
		NoLettersHint: inboxNoLettersHint,
	}
}
