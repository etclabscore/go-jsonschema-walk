package walk

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-openapi/spec"
	"github.com/stretchr/testify/assert"
)

func newTestSchema() *spec.Schema {
	s := spec.Schema{
		SchemaProps: spec.SchemaProps{
			Type:  []string{"object"},
		},
	}
	s.AsWritable()
	return &s
}

// withSliceChild is a convenience function for adding an subschema to a schema slice-type field,
// in this case AnyOf (chosen arbitrarily)
func withSliceChild(s *spec.Schema) (added *spec.Schema) {
	if s.AnyOf == nil {
		s.AnyOf = []spec.Schema{}
	}
	ns := newTestSchema()
	s.AnyOf = append(s.AnyOf, *ns)
	return ns
}

// withMapChild is a convenience function for adding an subschema to a schema map-type field,
// in this case Properties (chosen arbitrarily)
func withMapChild(s *spec.Schema) (added *spec.Schema) {
	if s.Properties == nil {
		s.Properties = make(map[string]spec.Schema)
	}
	ns := newTestSchema()

	// Use nanosecond timestamp as property key, which will always be unique.
	// Persist the key as the schema title, so that we can look it up again
	// in the test.
	k := fmt.Sprintf("%d", time.Now().UnixNano())
	ns.WithTitle(k)
	s.Properties[k] = *ns
	return ns
}

// onSchema is the mock for the schema node mutation callback.
// It appends a "." to a schema's description, for want of a better way
// of tracking mutations.
// It also logs the schema relative to the walk for test readability.
func onSchema(t *testing.T, walker *Walker) func(sch *spec.Schema) error {
	return func(sch *spec.Schema) error {
		treePre := strings.Repeat("\t", walker.depth)
		if walker.depth == 0 {
			treePre = ""
		}
		sch.WithDescription(sch.Description + ".")
		t.Log(spew.Sprintf("%d %s %+v", walker.depth, treePre, sch))
		return nil
	}
}

func TestWalker_Walk(t *testing.T) {
	t.Run("basic functionality", func(t *testing.T) {
		t.Run("root", func(t *testing.T) {
			walker := NewWalker()
			root := newTestSchema()

			err := walker.DepthFirst(root, onSchema(t, walker))
			assert.NoError(t, err)

			assert.Equal(t, 1, walker.iter)
			assert.Equal(t, ".", root.Description, "root touched")
		})
		t.Run("child", func(t *testing.T) {
			walker := NewWalker()

			root := newTestSchema()
			withSliceChild(root)

			err := walker.DepthFirst(root, onSchema(t, walker))
			assert.NoError(t, err)

			assert.Equal(t, 2, walker.iter, "iter")
			assert.Equal(t, ".", root.Description, "root touched")
			assert.Equal(t, ".", root.AnyOf[0].Description, "child touched")
		})
		t.Run("returns error", func(t *testing.T) {
			walker := NewWalker()

			root := newTestSchema()
			withSliceChild(root)

			err := walker.DepthFirst(root, func(node *spec.Schema) error {
				if walker.iter > 1 {
					return errors.New("myError")
				}
				return onSchema(t, walker)(node)
			})
			assert.Error(t, err)

			assert.Equal(t, 2, walker.iter)
			assert.Equal(t, "", root.Description, "root touched")
			assert.Equal(t, "", root.AnyOf[0].Description, "child touched")
		})
		t.Run("slice children", func(t *testing.T) {
			walker := NewWalker()

			root := newTestSchema()
			withSliceChild(root)
			withSliceChild(root)

			err := walker.DepthFirst(root, onSchema(t, walker))
			assert.NoError(t, err)

			assert.Equal(t, 3, walker.iter)
			assert.Equal(t, ".", root.Description, "root touched")
			assert.Equal(t, ".", root.AnyOf[0].Description, "child touched")
			assert.Equal(t, ".", root.AnyOf[1].Description, "child2 touched")
		})
		t.Run("map children", func(t *testing.T) {
			walker := NewWalker()

			root := newTestSchema()
			child := withMapChild(root)
			child2 := withMapChild(root) // iter:3

			err := walker.DepthFirst(root, onSchema(t, walker))
			assert.NoError(t, err)
			assert.Equal(t, 3, walker.iter)
			assert.Equal(t, ".", root.Description, "root touched")
			assert.Equal(t, ".", root.Properties[child.Title].Description, "child touched")
			assert.Equal(t, ".", root.Properties[child2.Title].Description, "child2 touched")
		})
	})
	t.Run("cycle detection", func(t *testing.T) {
		t.Run("basic", func(t *testing.T) {
			walker := NewWalker()

			root := newTestSchema()
			child := newTestSchema()
			child.AdditionalProperties = &spec.SchemaOrBool{true, root}
			root.AdditionalProperties = &spec.SchemaOrBool{true, child}

			err := walker.DepthFirst(root, onSchema(t, walker))
			assert.NoError(t, err)
			assert.Equal(t, 2, walker.iter)
			assert.Equal(t, ".", root.Description)
			assert.Equal(t, ".", root.AdditionalProperties.Schema.Description)
			assert.Len(t, walker.cycles(), 1)
		})
		t.Run("nested", func(t *testing.T) {
			walker := NewWalker()

			root := newTestSchema()
			child := newTestSchema()
			grandchild := newTestSchema()
			grandchild.AdditionalProperties = &spec.SchemaOrBool{true, root}
			child.AdditionalProperties = &spec.SchemaOrBool{true, grandchild}
			root.AdditionalProperties = &spec.SchemaOrBool{true, child}

			err := walker.DepthFirst(root, onSchema(t, walker))
			assert.NoError(t, err)
			assert.Equal(t, 3, walker.iter)
			assert.Equal(t, ".", root.Description)
			assert.Equal(t, ".", root.AdditionalProperties.Schema.Description)
			assert.Equal(t, ".", root.AdditionalProperties.Schema.AdditionalProperties.Schema.Description)
			assert.Len(t, walker.cycles(), 1)
		})
		t.Run("multiple", func(t *testing.T) {
			walker := NewWalker()

			root := newTestSchema()
			child1 := newTestSchema()
			child2 := newTestSchema()
			child2.AdditionalProperties = &spec.SchemaOrBool{true, root}
			child1.AdditionalProperties = &spec.SchemaOrBool{true, root}
			root.AdditionalProperties = &spec.SchemaOrBool{true, child1}
			root.AdditionalItems = &spec.SchemaOrBool{true, child2}

			err := walker.DepthFirst(root, onSchema(t, walker))
			assert.NoError(t, err)
			assert.Equal(t, 3, walker.iter)
			assert.Equal(t, ".", root.Description)
			assert.Equal(t, ".", root.AdditionalProperties.Schema.Description)
			assert.Equal(t, ".", root.AdditionalItems.Schema.Description)
			assert.Len(t, walker.cycles(), 2)
		})
	})
}
