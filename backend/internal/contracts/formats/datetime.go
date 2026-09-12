// Package formats adds the contract checks OpenAPI 3.0 cannot express on its own: stricter
// calendar validation for date-time values, and positional rules declared as schema extensions.
package formats

import (
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

func init() {
	// kin-openapi's default regexp accepts nonexistent dates such as February 30.
	openapi3.DefineStringFormatValidator("date-time", openapi3.NewCallbackValidator(func(value string) error {
		_, err := time.Parse(time.RFC3339Nano, value)
		return err
	}))
}
