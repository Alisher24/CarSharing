package httpapi

import "html/template"

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
