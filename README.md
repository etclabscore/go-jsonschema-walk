# go-jsonschema-walk

[![GoDoc](https://godoc.org/github.com/golang/gddo?status.svg)](https://godoc.org/github.com/etclabscore/go-jsonschema-walk)

Implements a [JSON Schema](https://json-schema.org/specification.html) [depth-first](https://en.wikipedia.org/wiki/Depth-first_search) walk with a callback function called 
once on each node, where a 'node' is any subschema within and including the original schema.

It implements pointer address cycle-detection and will gracefully avoid infinite recursions, although please keep in
mind that if your JSON schema graph contains a loop, other packages you may be using may _not_ implement this feature,
eg. `encoding/json#Marshal`.

This package uses https://github.com/go-openapi/spec as its JSON Schema type definition.

## Usage

```go
package main

import (
    "fmt"
    "log"

    "github.com/etclabscore/go-jsonschema-walk"
    "github.com/go-openapi/spec"
)

func main() {
    mySchema := &spec.Schema{
        SchemaProps:spec.SchemaProps{
        	Title: "thing1",
        	Type: []string{"object"},
        	AdditionalProperties: &spec.SchemaOrBool{
                Schema: &spec.Schema{
                    SchemaProps:spec.SchemaProps{
                        Title: "thing2",
                        Type:[]string{"string"},
                    },
                },
            },
        },
    }

    walker := walk.NewWalker()

    myCallback := func(s *spec.Schema) error {
        fmt.Println(s.Title)
        s.Description = "walker was here"
        return nil
    }

    err := walker.DepthFirst(mySchema, myCallback)
    if err != nil {
        log.Fatalln(err)
    }
}
```

## Tests

Tests follow Go convention, and can be run with `go test .`