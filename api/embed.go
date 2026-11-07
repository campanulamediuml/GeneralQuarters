package api

import _ "embed"

//go:embed openapi.yaml
var OpenAPI []byte

//go:embed docs.html
var Docs []byte
