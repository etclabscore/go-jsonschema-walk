package go_jsonschema_walk

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/go-openapi/spec"
)

type AnalysisT struct {
	recurseIter   int
	recursorStack []spec.Schema
	mutatedStack  []*spec.Schema

	/*
		@BelfordZ could modify 'prePostMap' to just postArray,
		and have isCycle just be "findSchema", returning the mutated schema if any.
		Look up orig--mutated by index/uniquetitle.
	*/
}

func NewAnalysisT() *AnalysisT {
	return &AnalysisT{
		recurseIter:   0,
		recursorStack: []spec.Schema{},
		mutatedStack:  []*spec.Schema{},
	}
}

func mustReadSchema(jsonStr string) *spec.Schema {
	s := &spec.Schema{}
	err := json.Unmarshal([]byte(jsonStr), &s)
	if err != nil {
		panic(fmt.Sprintf("read schema error: %v", err))
	}
	return s
}

func mustWriteJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err.Error())
	}
	return string(b)
}

func mustWriteJSONIndent(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err.Error())
	}
	return string(b)
}

func schemasAreEquivalent(s1, s2 *spec.Schema) bool {
	spec.ExpandSchema(s1, nil, nil)
	spec.ExpandSchema(s2, nil, nil)
	return reflect.DeepEqual(s1, s2)
}

// WalkDepthFirst runs a callback function on each leaf of a the JSON schema tree.
// It will return the first error it encounters.
func (a *AnalysisT) WalkDepthFirst(sch *spec.Schema, onNode func(node *spec.Schema) error) error {

	a.recurseIter++

	if sch == nil {
		return errors.New("WalkDepthFirst called on nil schema")
	}

	// Keep a pristine copy of the value on the recurse stack.
	// The incoming pointer value will be mutated.
	a.recursorStack = append(a.recursorStack, *mustReadSchema(mustWriteJSON(sch)))

	final := func(s *spec.Schema) error {
		err := onNode(s)
		a.mutatedStack = append([]*spec.Schema{s}, a.mutatedStack...)
		return err
	}

	// jsonschema slices.
	for i := 0; i < len(sch.AnyOf); i++ {
		err := a.WalkDepthFirst(&sch.AnyOf[i], onNode)
		if err != nil {
			return err
		}
	}
	for i := 0; i < len(sch.AllOf); i++ {
		err := a.WalkDepthFirst(&sch.AllOf[i], onNode)
		if err != nil {
			return err
		}
	}
	for i := 0; i < len(sch.OneOf); i++ {
		err := a.WalkDepthFirst(&sch.OneOf[i], onNode)
		if err != nil {
			return err
		}
	}

	// jsonschemama maps
	for k := range sch.Properties {
		v := sch.Properties[k]
		err := a.WalkDepthFirst(&v, onNode)
		if err != nil {
			return err
		}
		sch.Properties[k] = v
	}
	for k := range sch.PatternProperties {
		v := sch.PatternProperties[k]
		err := a.WalkDepthFirst(&v, onNode)
		if err != nil {
			return err
		}
		sch.PatternProperties[k] = v
	}

	// jsonschema special type
	if sch.Items == nil {
		return final(sch)
	}

	if sch.Items.Schema != nil {
		err := a.WalkDepthFirst(sch.Items.Schema, onNode)
		if err != nil {
			return err
		}
	} else {
		for i := range sch.Items.Schemas {
			err := a.WalkDepthFirst(&sch.Items.Schemas[i], onNode)
			if err != nil {
				return err
			}
		}
	}

	return final(sch)
}
