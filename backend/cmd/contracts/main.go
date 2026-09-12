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

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: contracts input.yaml output.json")
		os.Exit(2)
	}
	if err := bundle(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func bundle(input, output string) error {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = func(l *openapi3.Loader, uri *url.URL) ([]byte, error) {
		if uri.Scheme != "" && uri.Scheme != "file" || uri.Host != "" {
			return nil, fmt.Errorf("only local OpenAPI references are allowed")
		}
		return openapi3.ReadFromFile(l, uri)
	}
	spec, err := loader.LoadFromFile(input)
	if err != nil {
		return err
	}
	if spec.OpenAPI != "3.0.3" {
		return fmt.Errorf("expected OpenAPI 3.0.3")
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
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	return os.WriteFile(output, append(data, '\n'), 0o644)
}
