package httpapi

import (
	"embed"
	"html/template"
)

// The markup of the inbox pages, embedded in the binary: the stub stays one process with no build
// step and no volume of files, so its pages travel in it the way the migrations travel in the
// migrator.
//
//go:embed templates/*.html templates/*.css
var inboxMarkup embed.FS

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

// inboxTemplates is the markup of the inbox, parsed once as the process starts, and inboxStyles is
// the stylesheet every page of it states. Markup that cannot be parsed is a defect of this build
// rather than a condition of a request, so it fails here instead of in front of a reader.
var (
	inboxTemplates = template.Must(
		template.New(inboxLayoutTemplate).ParseFS(inboxMarkup, "templates/*.html"))
	inboxStyles = template.CSS(readInboxStylesheet())
)

// readInboxStylesheet reads the one stylesheet of the pages out of the binary. The file is part of
// this build, so not finding it is a defect of the build rather than a condition of a request.
func readInboxStylesheet() string {
	stylesheet, err := inboxMarkup.ReadFile(inboxStylesheet)
	if err != nil {
		panic(err)
	}
	return string(stylesheet)
}

// inboxDocument is the shape every page of the inbox is drawn in: the layout the pages share, filled
// in with the page's own title, summary and content, and the labels and addresses both are read by.
type inboxDocument struct {
	Title   string
	Summary string
	Styles  template.CSS
	Content template.HTML
	Labels  pageLabels
	JSON    string
	Current string
	Back    string
	Letter  bool
}

// pageContent is what a page's own template is filled in with: what the page read, and the labels it
// states around it.
type pageContent struct {
	Labels pageLabels
	Data   any
}

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
