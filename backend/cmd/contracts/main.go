// Command contracts resolves local OpenAPI references for the pinned generators.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/Alisher24/CarSharing/backend/internal/contracts/formats"
	"github.com/getkin/kin-openapi/openapi3"
)

// The version every source contract declares, and the permissions of the directory and file this
// command writes. Both are read-only outputs of the build, so they are not the place for a
// deliberately wider mode.
const (
	openAPIVersion = "3.0.3"

	outputDirectoryMode = 0o755
	outputFileMode      = 0o644
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: contracts input.yaml output.json")
		os.Exit(2)
	}
	if err := inlineExternalRefs(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// inlineExternalRefs loads a source contract together with everything it refers to and writes one
// self-contained document, which is the form the pinned generators read.
func inlineExternalRefs(input, output string) error {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(l *openapi3.Loader, uri *url.URL) ([]byte, error) {
		if !isLocalReference(uri) {
			return nil, fmt.Errorf("only local OpenAPI references are allowed")
		}
		return openapi3.ReadFromFile(l, uri)
	}
	spec, err := loader.LoadFromFile(input)
	if err != nil {
		return err
	}
	if spec.OpenAPI != openAPIVersion {
		return fmt.Errorf("expected OpenAPI %s", openAPIVersion)
	}
	if err := spec.Validate(context.Background()); err != nil {
		return err
	}
	spec.InternalizeRefs(context.Background(), func(_ *openapi3.T, ref openapi3.ComponentRef) string {
		parts := strings.Split(ref.RefString(), "/")
		return parts[len(parts)-1]
	})
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), outputDirectoryMode); err != nil {
		return err
	}
	return os.WriteFile(output, append(data, '\n'), outputFileMode)
}

// isLocalReference reports whether a reference points inside the source tree. A scheme other than
// file, or any host at all, would make the build depend on a document this repository does not own.
func isLocalReference(uri *url.URL) bool {
	remoteScheme := uri.Scheme != "" && uri.Scheme != "file"
	return !remoteScheme && uri.Host == ""
}
