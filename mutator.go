package go_jsonschema_traverse

import "github.com/go-openapi/spec"

type Mutator interface {
	OnSchema(s *spec.Schema) error
}
