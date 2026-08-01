package api

import _ "embed"

// OpenAPISpec is the public API contract served by the API process.
//
//go:embed openapi.yaml
var OpenAPISpec []byte
