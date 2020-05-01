package walk

import (
	"errors"
	"reflect"

	"github.com/go-openapi/spec"
)

// errCycle is returned when a cycle is detected.
// A cycle is when fields (or fields of fields... of fields)
// are found to contain duplicate addresses.
var errCycle = errors.New("cycle detected")

// errYesCycleNo tells us if the error is not nil and is not a cycle indicator.
func errYesCycleNo(err error) bool {
	return err != nil && err != errCycle
}

type Walker struct {
	iter     int
	depth    int
	pointers map[uintptr]int
	cycle    []cycle
}

type cycle struct {
	iter, depth int
}

func (w *Walker) cycles() []cycle {
	return w.cycle
}

func NewWalker() *Walker {
	return &Walker{
		depth:    -1,
		pointers: make(map[uintptr]int),
	}
}

// DepthFirst runs a mutating callback function on each node of a JSON schema graph.
// It will return the first error it encounters.
func (w *Walker) DepthFirst(root *spec.Schema, onNode func(node *spec.Schema) error) error {

	// Remove pointers at or below the current depth from map used to detect
	// circular refs.
	for k, depth := range w.pointers {
		if depth >= w.depth {
			delete(w.pointers, k)
		}
	}

	// Detect cycles.
	ptr := reflect.ValueOf(root).Pointer()
	if pDepth, ok := w.pointers[ptr]; ok && pDepth < w.depth {
		w.cycle = append(w.cycle, cycle{w.iter, w.depth})
		return errCycle
	}
	w.pointers[ptr] = w.depth

	w.iter++
	w.depth++
	defer func() {
		w.depth--
	}()

	for i := 0; i < len(root.AnyOf); i++ {
		err := w.DepthFirst(&root.AnyOf[i], onNode)
		if errYesCycleNo(err) {
			return err
		}
	}
	for i := 0; i < len(root.AllOf); i++ {
		err := w.DepthFirst(&root.AllOf[i], onNode)
		if errYesCycleNo(err) {
			return err
		}
	}
	for i := 0; i < len(root.OneOf); i++ {
		err := w.DepthFirst(&root.OneOf[i], onNode)
		if errYesCycleNo(err) {
			return err
		}
	}

	for k := range root.Properties {
		v := root.Properties[k]
		err := w.DepthFirst(&v, onNode)
		if errYesCycleNo(err) {
			return err
		}
		root.Properties[k] = v
	}
	for k := range root.PatternProperties {
		v := root.PatternProperties[k]
		err := w.DepthFirst(&v, onNode)
		if errYesCycleNo(err) {
			return err
		}
		root.PatternProperties[k] = v
	}

	if root.AdditionalProperties != nil && root.AdditionalProperties.Allows && root.AdditionalProperties.Schema != nil {
		err := w.DepthFirst(root.AdditionalProperties.Schema, onNode)
		if errYesCycleNo(err) {
			return err
		}
	}

	if root.AdditionalItems != nil && root.AdditionalItems.Allows && root.AdditionalItems.Schema != nil {
		err := w.DepthFirst(root.AdditionalItems.Schema, onNode)
		if errYesCycleNo(err) {
			return err
		}
	}

	if root.Items == nil {
		return onNode(root)
	}

	if root.Items.Schema != nil {
		err := w.DepthFirst(root.Items.Schema, onNode)
		if errYesCycleNo(err) {
			return err
		}
	} else {
		for i := range root.Items.Schemas {
			err := w.DepthFirst(&root.Items.Schemas[i], onNode)
			if errYesCycleNo(err) {
				return err
			}
		}
	}

	return onNode(root)
}
