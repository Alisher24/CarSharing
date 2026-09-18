package httpapi

import "testing"

// The routes of the human inbox, and what each address of it is. Every address of the surface is
// matched by segment: a path with more of itself than the pattern states is the unknown page it is,
// and one that merely begins like a page of the surface is not that page.
func TestTheRoutesOfThePagesMatchTheirAddresses(t *testing.T) {
	pattern, captures := "/messages/{id}", []string{inboxIdentifierCapture}
	for _, matched := range []struct {
		name     string
		path     string
		pattern  string
		captures []string
	}{
		{name: "the list", path: "/", pattern: inboxPath},
		{name: "the list with a cursor", path: "/", pattern: inboxPath},
		{name: "one letter", path: "/messages/" + storedLetterID(1), pattern: pattern, captures: captures},
	} {
		t.Run(matched.name, func(t *testing.T) {
			found, fits := inboxRouteMatch(matched.pattern, matched.path)
			if !fits {
				t.Fatalf("%s does not fit %s", matched.path, matched.pattern)
			}
			for _, name := range matched.captures {
				if found[name] == "" {
					t.Errorf("%s captured no %s", matched.pattern, name)
				}
			}
		})
	}

	for _, refused := range []struct {
		name    string
		pattern string
		path    string
	}{
		{name: "a path that only begins like the list", pattern: inboxPath, path: "/inbox"},
		{name: "a path that only begins like a letter", pattern: pattern, path: "/messages" + storedLetterID(1)},
		{name: "a letter of a letter", pattern: pattern, path: "/messages/" + storedLetterID(1) + "/body"},
		{name: "a letter named by nothing", pattern: pattern, path: "/messages/"},
	} {
		t.Run(refused.name, func(t *testing.T) {
			if _, fits := inboxRouteMatch(refused.pattern, refused.path); fits {
				t.Errorf("%s fits %s", refused.path, refused.pattern)
			}
		})
	}
}

// The namespace of the contract is derived from the paths the specification declares: a request under
// it belongs to the boundary even when the contract declares no such path, and a request outside it
// never does. A prefix answers at a segment boundary only, so a path that merely begins with the same
// letters is the page it is.
func TestTheNamespaceOfAContractIsDerivedFromItsPaths(t *testing.T) {
	spec, err := mailstubSpec(inboxMailListener, false)
	if err != nil {
		t.Fatal(err)
	}
	contract := newContractPrefixes(spec)
	for _, owned := range []struct {
		name string
		path string
	}{
		{name: "the collection", path: inboxMessagesLink},
		{name: "a letter", path: inboxMessagesLink + "/" + storedLetterID(1)},
		{name: "a path under the same namespace that the contract does not declare", path: "/api/v1/unknown"},
	} {
		t.Run("the contract owns "+owned.name, func(t *testing.T) {
			if !contract.owns(owned.path) {
				t.Errorf("%s belongs to the page surface", owned.path)
			}
		})
	}
	for _, pagePath := range []struct {
		name string
		path string
	}{
		{name: "the list", path: inboxPath},
		{name: "a letter", path: inboxLetterPath(storedLetterID(1))},
		{name: "a path that only begins with the same letters", path: "/messagesXXX"},
		{name: "an unknown path", path: "/nowhere"},
	} {
		t.Run("the contract owns not "+pagePath.name, func(t *testing.T) {
			if contract.owns(pagePath.path) {
				t.Errorf("%s belongs to the boundary", pagePath.path)
			}
		})
	}
}
