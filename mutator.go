package walk

import "github.com/go-openapi/spec"

type Mutator interface {
	OnSchema(s *spec.Schema) error
}
