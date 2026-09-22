package httpapi

import "html/template"

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
