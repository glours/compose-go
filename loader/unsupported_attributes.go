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
	"cmp"
	"slices"

	"github.com/compose-spec/compose-go/v2/tree"
)

// UnsupportedAttribute is a single instance of a caller-supplied
// UnsupportedAttributePattern matching the loaded compose model.
type UnsupportedAttribute struct {
	// Path locates the match in the loaded model, e.g. "services.web.deploy.mode".
	Path tree.Path
	// Value is whatever Path resolved to: a scalar, a map, or a list item,
	// depending on the depth the matching pattern targeted.
	Value any
}

// UnsupportedAttributePattern identifies a compose-file attribute that is
// schema-valid but not honored by the caller's runtime. compose-go does not
// know what any given runtime supports: the list of patterns is supplied by
// the caller.
type UnsupportedAttributePattern struct {
	// Path is a tree.Path pattern (may use tree.PathMatchAll) identifying
	// the node to inspect.
	Path tree.Path
	// Detect reports whether the value found at Path is unsupported. Nil
	// means presence alone is unsupported. Detect may target an
	// intermediate map node (not just a scalar leaf) to inspect sibling
	// fields, e.g. matching a whole `ports` entry to flag only `mode: host`.
	//
	// Once any pattern's Path matches a node, the walk stops descending into
	// that node's children regardless of what Detect returns: a pattern
	// targeting a child of an already-matched node will never be evaluated
	// for that occurrence.
	Detect func(value any) bool
}

// UnsupportedAttributesCheck bundles a set of UnsupportedAttributePattern
// with the callback invoked once, after loading, with every match found
// (possibly empty).
type UnsupportedAttributesCheck struct {
	Patterns []UnsupportedAttributePattern
	Report   func([]UnsupportedAttribute)
}

// WithUnsupportedAttributesCheck registers a set of UnsupportedAttributePattern
// to be evaluated against the loaded model. report is invoked once, after
// loading, with every match found (possibly empty).
func WithUnsupportedAttributesCheck(patterns []UnsupportedAttributePattern, report func([]UnsupportedAttribute)) func(*Options) {
	return func(opts *Options) {
		opts.UnsupportedAttributesCheck = &UnsupportedAttributesCheck{Patterns: patterns, Report: report}
	}
}

// detectUnsupportedAttributes walks dict and returns every match against
// patterns, ordered by Path. The walk itself visits map keys in Go's
// randomized order, so results are sorted here for deterministic output.
func detectUnsupportedAttributes(dict map[string]any, patterns []UnsupportedAttributePattern) []UnsupportedAttribute {
	findings := walkUnsupportedAttributes(dict, tree.NewPath(), patterns)
	slices.SortFunc(findings, func(a, b UnsupportedAttribute) int {
		return cmp.Compare(a.Path, b.Path)
	})
	return findings
}

func walkUnsupportedAttributes(value any, p tree.Path, patterns []UnsupportedAttributePattern) []UnsupportedAttribute {
	matched := false
	for _, pattern := range patterns {
		if !p.Matches(pattern.Path) {
			continue
		}
		matched = true
		if pattern.Detect == nil || pattern.Detect(value) {
			return []UnsupportedAttribute{{Path: p, Value: value}}
		}
	}
	if matched {
		// one or more patterns targeted this exact node but none flagged it:
		// don't descend any further into a node the caller already inspected
		// as a whole.
		return nil
	}
	var findings []UnsupportedAttribute
	switch v := value.(type) {
	case map[string]any:
		for k, e := range v {
			findings = append(findings, walkUnsupportedAttributes(e, p.Next(k), patterns)...)
		}
	case []any:
		for _, e := range v {
			findings = append(findings, walkUnsupportedAttributes(e, p.Next(tree.PathMatchList), patterns)...)
		}
	}
	return findings
}
