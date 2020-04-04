package go_jsonschema_traverse

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-openapi/spec"
	_ "github.com/stretchr/testify"
	"github.com/golang/mock/gomock"
)

func BenchmarkJSONMarshalDeterminism(b *testing.B) {
	v := `{
		"type": "object",
		"properties": {
		"foo": {},
		"bar": {},
		"baz": {}
	}`
	sch := mustReadSchema(v)
	v = mustWriteJSON(sch)
	for i := 0; i < b.N; i++ {
		got := mustWriteJSON(sch)
		if v != got {
			b.Fatal("bad")
		}
		sch = mustReadSchema(got)
	}
}

type schemaMatcher struct {
	s *spec.Schema
}

func newSchemaMatcherFromJSON(s *spec.Schema) schemaMatcher {
	return schemaMatcher{s: s}
}

func (s schemaMatcher) Matches(s2i interface{}) bool {
	s2, cast := s2i.(*spec.Schema)
	if !cast {
		return false
	}
	return schemasAreEquivalent(s.s, s2)
}

func (s schemaMatcher) String() string {
	return fmt.Sprintf("%v", s.s)
}

type traverseTestOptions struct {
	UniqueOnly bool
}

func traverseTest(t *testing.T, prop string, options *traverseTestOptions, sch *spec.Schema, expectCallTotal int) {

	runTest := func(a *AnalysisT, prop string, sch *spec.Schema, expectCallTotal int) {
		testController := gomock.NewController(t)
		mockMutator := NewMockMutator(testController)

		mutatorCalledN := 0
		fmt.Printf("'%s' unique=%v schema=\n%s\n", prop, options.UniqueOnly, mustWriteJSONIndent(sch))

		// Unique pattern is provided as an option, with the logic
		// provided here as a convenience.
		// This allows tests to expectations for unique node counts (fairly human readable),
		// compared to total node counts.
		registryUniq := make(map[string]*spec.Schema)
		registryDupe := make(map[string]*spec.Schema)
		if options.UniqueOnly {
			a.WalkDepthFirst(sch, func(s *spec.Schema) error {
				// If the key already exist in the registry
				k := mustWriteJSON(s)
				if _, ok := registryUniq[k]; ok {
					delete(registryUniq, k)
					registryDupe[k] = s
				} else {
					registryUniq[k] = s
				}
				return nil
			})
		}

		a.WalkDepthFirst(sch, func(s *spec.Schema) error {
			if options.UniqueOnly {
				_, ok := registryUniq[mustWriteJSON(s)]
				if !ok {
					return nil
				}
			}
			mutatorCalledN++
			fmt.Printf("'%s' unique=%v mutatorCalledN=%d, a.recurseIter=%d %s %s\n", prop, options.UniqueOnly, mutatorCalledN, a.recurseIter, strings.Repeat(" .", a.recurseIter-mutatorCalledN), mustWriteJSON(s))
			matcher := newSchemaMatcherFromJSON(s)
			mockMutator.EXPECT().OnSchema(matcher).Times(1)
			return mockMutator.OnSchema(s)
		})

		if mutatorCalledN != expectCallTotal {
			t.Errorf("bad mutator call total: '%s' unique=%v want=%d got=%d", prop, options.UniqueOnly, expectCallTotal, mutatorCalledN)
		} else {
			fmt.Printf("'%s' unique=%v OK: mutatorCalledN %d/%d\n", prop, options.UniqueOnly, mutatorCalledN, expectCallTotal)
		}
		fmt.Println()
		testController.Finish()
	}

	if options == nil {
		options = &traverseTestOptions{}
	}

	a := NewAnalysisT()
	prop = t.Name() + ":" + prop
	runTest(a, prop, sch, expectCallTotal)
}

//
//func TestSchemaItems(t *testing.T) {
//
//	raw := `{
//			  "title": "1-top",
//			  "type": "object",
//			  "properties": {
//				"foo": {
//				  "title": "2",
//				  "items": [
//					{
//					  "title": "3",
//					  "type": "array",
//					  "items": {
//						"title": "1-top",
//						"type": "object",
//						"properties": {
//						  "foo": {
//							"title": "2",
//							"items": [
//							  {
//								"title": "3",
//								"type": "array",
//								"items": {
//								  "title": "4-maxdepth"
//								}
//							  }
//							]
//						  }
//						}
//					  }
//					}
//				  ]
//				}
//			  }
//			}`
//
//	sch := mustReadSchema(raw)
//
//	if sch.Properties["foo"].Items == nil {
//		t.Fatal("bad")
//	}
//	if sch.Properties["foo"].Items.Len() != 1 {
//		t.Fatal("bad")
//	}
//}

func TestAnalysisT_WalkDepthFirst1(t *testing.T) {

	type traverseTestsOptsExpect map[*int]*traverseTestOptions
	uniqueOpts := &traverseTestOptions{UniqueOnly: true}
	pint := func(i int) *int {
		return &i
	}

	runTraverseTestWithOptsExpect := func(t *testing.T, prop string, oe traverseTestsOptsExpect, sch *spec.Schema) {
		for k := range oe {

			// You can limit the test scenarios that run with a whitelist on prop match.
			//if prop != "chained" {
			//	return
			//}

			v := oe[k]
			traverseTest(t, prop, v, sch, *k)
		}
	}

	t.Run("basic functionality", func(t *testing.T) {
		newBasicSchema := func(prop string, any interface{}) *spec.Schema { // Ternary default set
			var s *spec.Schema
			if any == nil {
				s = mustReadSchema(fmt.Sprintf(`{"%s": [{}, {}]}`, prop))
			} else {
				s = mustReadSchema(fmt.Sprintf(`{"%s": %s}`, prop, mustWriteJSON(any)))
			}
			return s
		}

		for _, s := range []string{"anyOf", "allOf", "oneOf"} {
			runTraverseTestWithOptsExpect(t, s, traverseTestsOptsExpect{
				pint(3): nil,
				pint(1): uniqueOpts,
			}, newBasicSchema(s, nil))
		}

		runTraverseTestWithOptsExpect(t, "traverses items when items is ordered list", traverseTestsOptsExpect{
			pint(3): nil,
			pint(1): uniqueOpts,
		}, newBasicSchema("items", nil))

		runTraverseTestWithOptsExpect(t, "traverses items when items constrained to single schema", traverseTestsOptsExpect{
			pint(2): nil,
			pint(2): uniqueOpts,
		}, mustReadSchema(`{"items": {"a": {}, "b": {}}}`))

		// This test is a good example of behavior.
		// The schema at properties.a is equivalent to the one at properties.b
		// If the Traverser uses schema equivalence to abort traversal, then
		// equivalent schemas at multiple paths will not be called.
		runTraverseTestWithOptsExpect(t, "traverses properties", traverseTestsOptsExpect{
			pint(3): nil,
			pint(1): uniqueOpts,
		}, mustReadSchema(`{"properties": {"a": {}, "b": {}}}`))
	})

	t.Run("cycle detection", func(t *testing.T) {

		// Expect that
		runTraverseTestWithOptsExpect(t, "basic", traverseTestsOptsExpect{
			pint(3): nil,
			pint(3): uniqueOpts,
		}, func() *spec.Schema {

			raw := `{
			  "type": "object",
			  "properties": {
				"foo": {}
			  }
			}`

			s := mustReadSchema(raw)
			s.Properties["foo"] = *mustReadSchema(raw)
			return s

		}())

		/*
			This test will cause a stack overflow because it uses
			the parent as it's own child.
			This case is allowed to overflow because
			an overflow is caused if the same case is applied to json.Marshal.
			If the standard library fails in this case, then we can too.
		*/
		//runTraverseTestWithOptsExpect(t, "basic-ref", traverseTestsOptsExpect{
		//	pint(3): nil,
		//	pint(3): uniqueOpts,
		//}, func() *spec.Schema {
		//
		//	raw := `{"type": "object", "properties": {"foo": {}}}`
		//	s := mustReadSchema(raw)
		//	s.Properties["foo"] = *s
		//	return s
		//
		//}())

		runTraverseTestWithOptsExpect(t, "chained", traverseTestsOptsExpect{
			pint(7): nil,
			pint(7): uniqueOpts,
		}, func() *spec.Schema {

			raw := `{
			  "title": "1-top",
			  "type": "object",
			  "properties": {
				"foo": {
				  "title": "2",
				  "items": [
					{
					  "title": "3",
					  "type": "array",
					  "items": {
						"title": "4-maxdepth"
					  }
					}
				  ]
				}
			  }
			}`

			s := mustReadSchema(raw)
			s.Properties["foo"].Items.Schemas[0].Items.Schema = mustReadSchema(raw)
			return s

		}())

		runTraverseTestWithOptsExpect(t, "chained in media res", traverseTestsOptsExpect{
			pint(8): nil,
			pint(8): uniqueOpts,
		}, func() *spec.Schema {
			raw := `{
			  "title": "1",
			  "type": "object",
			  "properties": {
				"foo": {
				  "title": "2",
				  "anyOf": [
					{
					  "title": "3",
					  "type": "array",
					  "items": {
						"title": "4",
						"properties": {
						  "baz": {
							"title": "5"
						  }
						}
					  }
					}
				  ]
				}
			  }
			}`

			s := mustReadSchema(raw)
			s.Properties["foo"].AnyOf[0].Items.Schema.Properties["baz"] = mustReadSchema(raw).Properties["foo"]
			return s
		}())

		runTraverseTestWithOptsExpect(t, "chained in media res different branch", traverseTestsOptsExpect{
			pint(17): nil,
			pint(11): uniqueOpts,
		}, func() *spec.Schema {
			raw := `{
			  "title": "1",
			  "type": "object",
			  "properties": {
				"foo": {
				  "title": "2",
				  "anyOf": [
					{
					  "title": "3",
					  "type": "array",
					  "items": {
						"title": "4",
						"properties": {
						  "baz": {
							"title": "5"
						  }
						}
					  }
					}
				  ]
				},
				"bar": {
				  "title": "6",
				  "type": "object",
				  "allOf": [
					{
					  "title": "7",
					  "type": "object",
					  "properties": {
						"baz": {
						  "title": "8"
						}
					  }
					}
				  ]
				}
			  }
			}`
			s := mustReadSchema(raw)
			s.Properties["foo"].AnyOf[0].Items.Schema.Properties["baz"] = *mustReadSchema(raw)
			s.Properties["bar"].AllOf[0].Properties["baz"] = mustReadSchema(raw).Properties["foo"].AnyOf[0]
			return s
		}())

		runTraverseTestWithOptsExpect(t, "multiple cycles", traverseTestsOptsExpect{
			pint(8): nil,
			pint(6): uniqueOpts,
		}, func() *spec.Schema {

			raw := `{
			  "title": "1",
			  "type": "object",
			  "properties": {
				"foo": {
				  "title": "2",
				  "anyOf": [
					{
					  "title": "3",
					  "type": "array",
					  "items": {
						"title": "4",
						"properties": {
						  "baz": {
							"title": "5"
						  }
						}
					  }
					}
				  ]
				},
				"bar": {
				  "title": "6",
				  "type": "object",
				  "allOf": [
					{
					  "title": "7",
					  "type": "object",
					  "properties": {
						"baz": {
						  "title": "8"
						}
					  }
					}
				  ]
				}
			  }
			}`

			s := mustReadSchema(raw)
			s.Properties["bar"].AllOf[0].Properties["baz"] = mustReadSchema(raw).Properties["foo"].AnyOf[0].Items.Schema.Properties["baz"]
			bar := s.Properties["bar"]
			bar.WithAllOf(*mustReadSchema(raw))
			bar.WithAllOf(mustReadSchema(raw).Properties["foo"].AnyOf[0].Items.Schemas...)
			return s
		}())
	})

	mutationTest := func(domain string, newAnalyst func() *AnalysisT, s *spec.Schema, mut func(*spec.Schema) error, checkFn func(*AnalysisT, *spec.Schema)) {

		fmt.Printf("mutation test '%s' Before @schema=\n%s\n", domain, mustWriteJSONIndent(s))

		a := newAnalyst()
		a.WalkDepthFirst(s, mut)
		fmt.Printf("mutation test '%s' After @schema=\n%s\n", domain, mustWriteJSONIndent(s))

		// Checks.
		checkFn(a, s)
		if len(a.recursorStack) != len(a.mutatedStack) {
			fmt.Printf("mutation test '%s' After @schema=\n%s\n", domain, mustWriteJSONIndent(s))
			t.Error("bad")
		}

		// Logs.
		for i, v := range a.recursorStack {
			fmt.Println("orig=", mustWriteJSON(v), "mutated=", mustWriteJSON(a.mutatedStack[i]))
		}
	}

	// Test that every node in the graph gets mutated gets mutated.
	t.Run("mutator function mutates schema values", func(t *testing.T) {

		const mockDescription = "baz"
		descriptionMutator := func(s *spec.Schema) error {
			s.Description = mockDescription
			return nil
		}

		basicSchema := mustReadSchema(`{"title": "object", "properties": {"foo": {"title": "bar"}}}`)
		mutationTest("basic", NewAnalysisT, basicSchema, descriptionMutator, func(aa *AnalysisT, s *spec.Schema) {
			if s.Description != mockDescription {
				t.Error("notbaz")
			}
			if s.Properties["foo"].Description != mockDescription {
				t.Error(".foo not baz")
			}
		})

		anyOfSchema := mustReadSchema(`
		{
		  "anyOf": [
			{
			  "type": "object",
			  "properties": {
				"foo": {}
			  }
			},
			{
			  "type": "object",
			  "properties": {
				"foo": {}
			  }
			}
		  ]
		}`)

		mutationTest("anyOf nonunique", NewAnalysisT, anyOfSchema, descriptionMutator, func(aa *AnalysisT, s *spec.Schema) {
			if s.Description != mockDescription {
				t.Error(". not baz")
			}
			if s.AnyOf[0].Description != mockDescription {
				t.Error(".anyOf[0] not baz")
			}
			if s.AnyOf[0].Properties["foo"].Description != mockDescription {
				t.Error(".anyOf[0].foo not baz")
			}
			if s.AnyOf[1].Description != mockDescription {
				t.Error(".anyOf[1] not baz")
			}
			if s.AnyOf[1].Properties["foo"].Description != mockDescription {
				t.Error(".anyOf[1].foo not baz")
			}
		})
	})

	// Demonstrates a usage pattern that only mutates unique nodes.
	t.Run("uniq mutations pattern test", func(t *testing.T) {
		anyOfSchema2 := mustReadSchema(`
		{
		  "anyOf": [
			{
			  "type": "object",
			  "properties": {
				"foo": {}
			  }
			},
			{
			  "type": "object",
			  "properties": {
				"foo": {}
			  }
			}
		  ]
		}`)

		registryUniq := make(map[string]*spec.Schema)
		registryDupe := make(map[string]*spec.Schema)
		descriptionMutatorCollectUniq := func(s *spec.Schema) error {
			k := mustWriteJSON(s)
			if _, ok := registryUniq[k]; ok {
				delete(registryUniq, k)
				registryDupe[k] = s
				return nil
			}
			registryUniq[k] = s
			return nil
		}
		const mockDescription = "baz"
		descriptionMutatorIfUniq := func(s *spec.Schema) error {
			if _, ok := registryUniq[mustWriteJSON(s)]; !ok {
				return nil
			}
			s.Description = mockDescription
			return nil
		}

		a := NewAnalysisT()

		// Pass 1: Collect the dictionaries of unique and duped schemas for the whole graph.
		a.WalkDepthFirst(anyOfSchema2, descriptionMutatorCollectUniq)

		// Pass 2: Run the mutation test using the -IfUniq mutator, expecting that
		// only schemas identified in Pass 1 as unique will be mutated.
		mutationTest("anyOf unique", func() *AnalysisT {
			a := NewAnalysisT()
			return a
		}, anyOfSchema2, descriptionMutatorIfUniq, func(aa *AnalysisT, s *spec.Schema) {
			if s.Description != mockDescription {
				t.Error(". not baz")
			}
			if s.AnyOf[0].Description == mockDescription {
				t.Error(".anyOf[0] is baz")
			}
			if s.AnyOf[0].Properties["foo"].Description == mockDescription {
				t.Error(".anyOf[0].foo is baz")
			}
			// PTAL: Since the mutator runs from the bottom-up (depth-first traversal),
			// the
			if s.AnyOf[1].Description == mockDescription {
				t.Error(".anyOf[1] is baz")
			}
			if s.AnyOf[1].Properties["foo"].Description == mockDescription {
				t.Error(".anyOf[1].foo is baz")
			}
		})
	})

	// Distinct schemas represents at most one of each schema object.
	// This is different from unique schemas, since schemas with dupes
	// are included one time, instead of zero times.
	t.Run("distinct schemas memoization pattern", func(t *testing.T) {
		s := func() *spec.Schema {
			raw := `{
			  "title": "1",
			  "type": "object",
			  "properties": {
				"foo": {
				  "title": "2",
				  "anyOf": [
					{
					  "title": "3",
					  "type": "array",
					  "items": {
						"title": "4",
						"properties": {
						  "baz": {
							"title": "5"
						  }
						}
					  }
					}
				  ]
				},
				"bar": {
				  "title": "6",
				  "type": "object",
				  "allOf": [
					{
					  "title": "7",
					  "type": "object",
					  "properties": {
						"baz": {
							"title": "5"
						},
						"baz2": {
							"title": "5"
						}
					  }
					}
				  ]
				}
			  }
			}`
			s := mustReadSchema(raw)
			return s
		}()

		a := NewAnalysisT()

		registry := make(map[string]*spec.Schema)

		a.WalkDepthFirst(s, func(s *spec.Schema) error {
			registry[mustWriteJSON(s)] = s
			return nil
		})

		if len(registry) != 7 {
			t.Fatal("bad")
		}

		n := 0
		for k := range registry {
			fmt.Println(n, k)
			n++
		}
	})

	t.Run("walk order", func(t *testing.T) {
		raw := `{
		  "type": "object",
		  "properties": {
			"foo": {
			  "type": "object",
			  "properties": {
				"bar": {
				  "title": "baz"
				}
			  }
			}
		  }
		}`

		s := mustReadSchema(raw)
		a := NewAnalysisT()

		a.WalkDepthFirst(s, func(node *spec.Schema) error {
			node.Description = "touched"
			return nil
		})

		if l := len(a.recursorStack); l != 3 {
			for _, s := range a.recursorStack {
				fmt.Println(mustWriteJSON(s))
			}
			t.Fatal("bad recursor stack", l)
		}
		if len(a.recursorStack) != len(a.mutatedStack) {
			for _, m := range a.mutatedStack {
				fmt.Println(mustWriteJSON(m))
			}
			t.Fatal("bad mutated stack", len(a.mutatedStack))
		}

		checkEquivalent := func(s1, s2 *spec.Schema) {
			fmt.Println(mustWriteJSON(s1), mustWriteJSON(s2))
			if !schemasAreEquivalent(s1, s2) {
				t.Error("not eq")
			}
		}
		// Walk ordering.
		for i, item := range a.recursorStack {
			switch i {
			case 0:
				checkEquivalent(&item, mustReadSchema(raw))
			case 1:
				raw := `{
						  "type": "object",
						  "properties": {
							"bar": {
							  "title": "baz"
							}
						  }
						}`
				checkEquivalent(&item, mustReadSchema(raw))
			case 2:
				raw := `{"title": "baz"}`
				checkEquivalent(&item, mustReadSchema(raw))
			}
		}

		// Walk orig/mutations.
		for i := range a.recursorStack {
			fmt.Println(mustWriteJSON(a.recursorStack[i]), " => ", mustWriteJSON(a.mutatedStack[i]))
		}
	})
}

// TestDemoSchemaExpand is just an demonstration to stdout of what jsonschema.ExpandSchema does.
func TestDemoSchemaExpand(t *testing.T) {
	raw := `{
	  "title": "1",
	  "type": "object",
	  "properties": {
		"foo": {
		  "$ref": "#/definitions/foo"
		}
	  },
	  "definitions": {
		"foo": {
		  "title": "myfoo"
		}
	  }
	}`

	s := mustReadSchema(raw)

	fmt.Println("Before", mustWriteJSONIndent(s))

	s.AsWritable()
	err := spec.ExpandSchema(s, s, nil)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("After", mustWriteJSONIndent(s))
}

/*











































 */
