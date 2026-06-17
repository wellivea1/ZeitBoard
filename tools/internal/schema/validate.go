// Package schema validates the versioned JSON Schema contracts and the
// synthetic fixtures against them. It is the Go replacement for the former
// check-jsonschema (Python) dependency and runs the same checks:
// schema well-formedness (metaschema), fixture validation, and the sample
// configuration.
package schema

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	metaschemaURL = "https://json-schema.org/draft/2020-12/schema"
	resourceBase  = "mem:///contracts/v1/"
)

// fixturePairs mirrors scripts/validate-contracts.sh, plus the refused
// estimate fixture (validated against the same phase-estimate schema) for
// fuller coverage.
var fixturePairs = []struct{ schema, fixture string }{
	{"observation-set.schema.json", "observations.json"},
	{"correction-set.schema.json", "corrections.json"},
	{"sync-batch.schema.json", "sync-batch.json"},
	{"assistant-action.schema.json", "assistant-action.json"},
	{"phase-estimate.schema.json", "phase-estimate.json"},
	{"phase-estimate.schema.json", "phase-estimate-refused.json"},
	{"schedule-request.schema.json", "schedule-request.json"},
	{"schedule-proposals.schema.json", "schedule-proposals.json"},
	{"share-profile.schema.json", "share-profile-default-deny.json"},
	{"share-profile.schema.json", "share-profile-allowlisted.json"},
	{"trusted-view.schema.json", "trusted-view-default-deny.json"},
	{"trusted-view.schema.json", "trusted-view.json"},
}

// Set holds the compiled contract schemas for a repository checkout.
type Set struct {
	contractsDir string
	compiler     *jsonschema.Compiler
	names        []string
	cache        map[string]*jsonschema.Schema
}

// Load reads every contracts/v1/*.schema.json into a compiler with format
// assertion enabled (so formats such as date-time are enforced, matching
// check-jsonschema).
func Load(root string) (*Set, error) {
	contractsDir := filepath.Join(root, "contracts", "v1")
	entries, err := os.ReadDir(contractsDir)
	if err != nil {
		return nil, fmt.Errorf("read contracts dir: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()

	set := &Set{
		contractsDir: contractsDir,
		compiler:     compiler,
		cache:        map[string]*jsonschema.Schema{},
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".schema.json") {
			continue
		}
		doc, err := loadJSON(filepath.Join(contractsDir, name))
		if err != nil {
			return nil, err
		}
		if err := compiler.AddResource(resourceBase+name, doc); err != nil {
			return nil, fmt.Errorf("add resource %s: %w", name, err)
		}
		set.names = append(set.names, name)
	}
	return set, nil
}

func (s *Set) schema(name string) (*jsonschema.Schema, error) {
	if sch, ok := s.cache[name]; ok {
		return sch, nil
	}
	sch, err := s.compiler.Compile(resourceBase + name)
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", name, err)
	}
	s.cache[name] = sch
	return sch, nil
}

// CheckMetaschemas validates every contract schema document against the
// draft 2020-12 metaschema (the equivalent of check-jsonschema
// --check-metaschema) and confirms each schema compiles.
func (s *Set) CheckMetaschemas() error {
	meta, err := s.compiler.Compile(metaschemaURL)
	if err != nil {
		return fmt.Errorf("compile metaschema: %w", err)
	}
	var errs []error
	for _, name := range s.names {
		doc, err := loadJSON(filepath.Join(s.contractsDir, name))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := meta.Validate(doc); err != nil {
			errs = append(errs, fmt.Errorf("%s is not a valid draft 2020-12 schema: %w", name, err))
		}
		if _, err := s.schema(name); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ValidateInstance validates an already-decoded instance against a named schema.
func (s *Set) ValidateInstance(schemaName string, instance any) error {
	sch, err := s.schema(schemaName)
	if err != nil {
		return err
	}
	return sch.Validate(instance)
}

// ValidateBytes validates raw JSON against a named schema.
func (s *Set) ValidateBytes(schemaName string, data []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return s.ValidateInstance(schemaName, doc)
}

// ValidateFile validates a JSON file against a named schema.
func (s *Set) ValidateFile(schemaName, path string) error {
	doc, err := loadJSON(path)
	if err != nil {
		return err
	}
	if err := s.ValidateInstance(schemaName, doc); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return nil
}

// ValidateAll runs the full contract suite: metaschema checks, every fixture,
// and the sample configuration.
func ValidateAll(root string) error {
	set, err := Load(root)
	if err != nil {
		return err
	}

	var errs []error
	if err := set.CheckMetaschemas(); err != nil {
		errs = append(errs, err)
	}

	testdataDir := filepath.Join(root, "testdata", "v1")
	for _, pair := range fixturePairs {
		if err := set.ValidateFile(pair.schema, filepath.Join(testdataDir, pair.fixture)); err != nil {
			errs = append(errs, err)
		}
	}

	if err := set.ValidateFile("config.schema.json", filepath.Join(root, "config.example.json")); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func loadJSON(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return doc, nil
}
