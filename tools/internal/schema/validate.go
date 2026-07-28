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

	"non24.app/tools/internal/fixtures"
)

const (
	metaschemaURL = "https://json-schema.org/draft/2020-12/schema"
)

// Set holds the compiled contract schemas for a repository checkout.
type Set struct {
	contractsDir string
	resourceBase string
	compiler     *jsonschema.Compiler
	names        []string
	cache        map[string]*jsonschema.Schema
}

// Load reads every contracts/v1/*.schema.json into a compiler with format
// assertion enabled (so formats such as date-time are enforced, matching
// check-jsonschema).
func Load(root string) (*Set, error) {
	return loadVersion(root, "v1")
}

func loadVersion(root, version string) (*Set, error) {
	if !validContractVersion(version) {
		return nil, fmt.Errorf("invalid contract version %q", version)
	}
	contractsDir := filepath.Join(root, "contracts", version)
	entries, err := os.ReadDir(contractsDir)
	if err != nil {
		return nil, fmt.Errorf("read contracts dir: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()

	set := &Set{
		contractsDir: contractsDir,
		resourceBase: "mem:///contracts/" + version + "/",
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
		if err := compiler.AddResource(set.resourceBase+name, doc); err != nil {
			return nil, fmt.Errorf("add resource %s: %w", name, err)
		}
		set.names = append(set.names, name)
	}
	return set, nil
}

func validContractVersion(version string) bool {
	if len(version) < 2 || version[0] != 'v' {
		return false
	}
	for _, char := range version[1:] {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (s *Set) schema(name string) (*jsonschema.Schema, error) {
	if sch, ok := s.cache[name]; ok {
		return sch, nil
	}
	sch, err := s.compiler.Compile(s.resourceBase + name)
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
// and the sample configuration. Fixture paths, versions, and schemas come from
// the generator manifest so generation and validation cannot silently diverge.
func ValidateAll(root string) error {
	var errs []error
	sets := make(map[string]*Set)
	generated, err := fixtures.Build()
	if err != nil {
		errs = append(errs, fmt.Errorf("build fixture manifest: %w", err))
		return errors.Join(errs...)
	}
	for _, entry := range generated {
		if _, loaded := sets[entry.Version]; loaded {
			continue
		}
		set, err := loadVersion(root, entry.Version)
		if err != nil {
			errs = append(errs, err)
			sets[entry.Version] = nil
			continue
		}
		sets[entry.Version] = set
		if err := set.CheckMetaschemas(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, entry := range generated {
		set := sets[entry.Version]
		if set == nil {
			continue
		}
		fixturePath := filepath.Join(root, filepath.FromSlash(entry.GeneratedPath()))
		if err := set.ValidateFile(entry.Schema, fixturePath); err != nil {
			errs = append(errs, err)
		}
	}

	v1 := sets["v1"]
	if v1 == nil {
		var err error
		v1, err = Load(root)
		if err != nil {
			errs = append(errs, err)
			return errors.Join(errs...)
		}
	}
	if err := v1.ValidateFile("config.schema.json", filepath.Join(root, "config.example.json")); err != nil {
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
