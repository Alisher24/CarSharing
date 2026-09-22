package httpapi

// pageContent is what a page's own template is filled in with: what the page read, and the labels it
// states around it.
type pageContent struct {
	Labels pageLabels
	Data   any
}
