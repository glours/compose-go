/*
   Copyright 2020 The Compose Specification Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package loader

import (
	"context"
	"testing"

	"github.com/compose-spec/compose-go/v2/tree"
	"github.com/compose-spec/compose-go/v2/types"
	"gotest.tools/v3/assert"
	is "gotest.tools/v3/assert/cmp"
)

func TestDetectUnsupportedAttributes_PathPresence(t *testing.T) {
	dict := map[string]any{
		"services": map[string]any{
			"web": map[string]any{
				"image": "nginx",
				"deploy": map[string]any{
					"mode": "replicated",
				},
			},
			"api": map[string]any{
				"image": "alpine",
			},
		},
	}
	patterns := []UnsupportedAttributePattern{
		{Path: tree.NewPath("services", "*", "deploy", "mode")},
	}

	findings := detectUnsupportedAttributes(dict, patterns)

	assert.Assert(t, is.Len(findings, 1))
	assert.Equal(t, findings[0].Path, tree.NewPath("services", "web", "deploy", "mode"))
	assert.Equal(t, findings[0].Value, "replicated")
}

func TestDetectUnsupportedAttributes_ValueConditional(t *testing.T) {
	dict := map[string]any{
		"services": map[string]any{
			"web": map[string]any{
				"ports": []any{
					map[string]any{"target": 80, "protocol": "tcp", "mode": "ingress"},
					map[string]any{"target": 53, "protocol": "udp", "mode": "host"},
				},
			},
		},
	}
	patterns := []UnsupportedAttributePattern{
		{
			Path: tree.NewPath("services", "*", "ports", "[]"),
			Detect: func(value any) bool {
				port, ok := value.(map[string]any)
				if !ok {
					return false
				}
				return port["mode"] == "host"
			},
		},
	}

	findings := detectUnsupportedAttributes(dict, patterns)

	assert.Assert(t, is.Len(findings, 1))
	assert.Equal(t, findings[0].Path, tree.NewPath("services", "web", "ports", "[]"))
	port := findings[0].Value.(map[string]any)
	assert.Equal(t, port["target"], 53)
}

func TestDetectUnsupportedAttributes_NoPatternsMatch(t *testing.T) {
	dict := map[string]any{
		"services": map[string]any{
			"web": map[string]any{"image": "nginx"},
		},
	}
	patterns := []UnsupportedAttributePattern{
		{Path: tree.NewPath("services", "*", "deploy", "mode")},
	}

	findings := detectUnsupportedAttributes(dict, patterns)

	assert.Assert(t, is.Len(findings, 0))
}

func TestDetectUnsupportedAttributes_DoesNotDescendIntoMatchedNode(t *testing.T) {
	// A pattern matching a whole map node (ports.[]) must not also fire a
	// second, narrower pattern nested underneath it: the first match wins
	// and stops that branch, mirroring validation.check's short-circuit.
	dict := map[string]any{
		"services": map[string]any{
			"web": map[string]any{
				"ports": []any{
					map[string]any{"target": 53, "protocol": "udp", "mode": "host"},
				},
			},
		},
	}
	patterns := []UnsupportedAttributePattern{
		{Path: tree.NewPath("services", "*", "ports", "[]")},
		{Path: tree.NewPath("services", "*", "ports", "[]", "mode")},
	}

	findings := detectUnsupportedAttributes(dict, patterns)

	assert.Assert(t, is.Len(findings, 1))
	assert.Equal(t, findings[0].Path, tree.NewPath("services", "web", "ports", "[]"))
}

func TestDetectUnsupportedAttributes_AllPatternsAtSamePathAreEvaluated(t *testing.T) {
	// Two independent patterns can target the same node (e.g. one flagging
	// an unsupported `condition`, another an unsupported `restart` value on
	// the same depends_on entry). An earlier pattern whose Detect rules out
	// this particular node must not prevent a later pattern from still
	// being evaluated against it.
	dict := map[string]any{
		"services": map[string]any{
			"web": map[string]any{
				"depends_on": map[string]any{
					"api": map[string]any{
						"condition": "service_started",
						"restart":   true,
					},
				},
			},
		},
	}
	patterns := []UnsupportedAttributePattern{
		{
			Path: tree.NewPath("services", "*", "depends_on", "*"),
			Detect: func(value any) bool {
				dep, _ := value.(map[string]any)
				return dep["condition"] == "unknown_condition"
			},
		},
		{
			Path: tree.NewPath("services", "*", "depends_on", "*"),
			Detect: func(value any) bool {
				dep, _ := value.(map[string]any)
				return dep["restart"] == true
			},
		},
	}

	findings := detectUnsupportedAttributes(dict, patterns)

	assert.Assert(t, is.Len(findings, 1))
	assert.Equal(t, findings[0].Path, tree.NewPath("services", "web", "depends_on", "api"))
}

func TestWithUnsupportedAttributesCheckOption(t *testing.T) {
	yaml := `
name: test
services:
  web:
    image: nginx
    deploy:
      mode: replicated
  api:
    image: alpine
`
	patterns := []UnsupportedAttributePattern{
		{Path: tree.NewPath("services", "*", "deploy", "mode")},
	}

	t.Run("reports findings from the loaded model", func(t *testing.T) {
		var reported []UnsupportedAttribute
		_, err := LoadWithContext(context.Background(), buildConfigDetails(yaml, nil),
			WithUnsupportedAttributesCheck(patterns, func(findings []UnsupportedAttribute) {
				reported = findings
			}),
		)
		assert.NilError(t, err)
		assert.Assert(t, is.Len(reported, 1))
		assert.Equal(t, reported[0].Path, tree.NewPath("services", "web", "deploy", "mode"))
	})

	t.Run("callback with no patterns runs but finds nothing", func(t *testing.T) {
		var reported []UnsupportedAttribute
		called := false
		_, err := LoadWithContext(context.Background(), buildConfigDetails(yaml, nil),
			WithUnsupportedAttributesCheck(nil, func(findings []UnsupportedAttribute) {
				called = true
				reported = findings
			}),
		)
		assert.NilError(t, err)
		assert.Assert(t, called, "report must still be invoked with an empty slice")
		assert.Assert(t, is.Len(reported, 0))
	})

	t.Run("findings are reported once even when the attribute comes from an included file", func(t *testing.T) {
		tmpdir := t.TempDir()
		mainYaml := `
name: test
include:
  - included.yaml
services:
  api:
    image: alpine
`
		includedYaml := `
services:
  web:
    image: nginx
    deploy:
      mode: replicated
`
		createFile(t, tmpdir, includedYaml, "included.yaml")
		path := createFile(t, tmpdir, mainYaml, "compose.yaml")

		var calls int
		var reported []UnsupportedAttribute
		_, err := LoadWithContext(context.Background(), types.ConfigDetails{
			WorkingDir:  tmpdir,
			ConfigFiles: []types.ConfigFile{{Filename: path}},
		},
			WithUnsupportedAttributesCheck(patterns, func(findings []UnsupportedAttribute) {
				calls++
				reported = findings
			}),
		)
		assert.NilError(t, err)
		assert.Equal(t, calls, 1)
		assert.Assert(t, is.Len(reported, 1))
		assert.Equal(t, reported[0].Path, tree.NewPath("services", "web", "deploy", "mode"))
	})
}
