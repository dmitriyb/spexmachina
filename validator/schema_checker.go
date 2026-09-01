package validator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// compiledSchemas holds precompiled JSON Schemas for project.json and module.json.
type compiledSchemas struct {
	project *jsonschema.Schema
	module  *jsonschema.Schema
}

// compileSchemas composes the project and module JSON Schemas from the
// given resolved profile and compiles them. Composition and compilation
// happen once per CheckSchema call and are reused for every file that call
// validates, so a spec of a hundred modules costs one compilation and one
// validation call per file. The result is not cached across calls: the
// composed schema depends on the profile the caller's spec directory
// resolves, which can differ from one call to the next.
func compileSchemas(profile *schema.Profile) (*compiledSchemas, error) {
	c := jsonschema.NewCompiler()

	projBytes, err := schema.ComposeProjectSchema(profile.ProjectNodeTypes(), profile.Edges)
	if err != nil {
		return nil, fmt.Errorf("validator: compose project schema: %w", err)
	}
	projDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(projBytes))
	if err != nil {
		return nil, fmt.Errorf("validator: parse project schema: %w", err)
	}
	if err := c.AddResource("project.schema.json", projDoc); err != nil {
		return nil, fmt.Errorf("validator: add project schema: %w", err)
	}

	modBytes, err := schema.ComposeModuleSchema(profile.ModuleNodeTypes(), profile.Edges)
	if err != nil {
		return nil, fmt.Errorf("validator: compose module schema: %w", err)
	}
	modDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(modBytes))
	if err != nil {
		return nil, fmt.Errorf("validator: parse module schema: %w", err)
	}
	if err := c.AddResource("module.schema.json", modDoc); err != nil {
		return nil, fmt.Errorf("validator: add module schema: %w", err)
	}

	projSchema, err := c.Compile("project.schema.json")
	if err != nil {
		return nil, fmt.Errorf("validator: compile project schema: %w", err)
	}
	modSchema, err := c.Compile("module.schema.json")
	if err != nil {
		return nil, fmt.Errorf("validator: compile module schema: %w", err)
	}

	return &compiledSchemas{project: projSchema, module: modSchema}, nil
}

// CheckSchema validates project.json and all module.json files in specDir
// against the JSON Schemas composed from the spec directory's resolved
// profile. It returns all violations found.
//
// The profile is resolved before any schema is compiled, so a malformed
// profile.json is reported as a single early failure naming the profile
// file, never as a cascade of conformance errors against a half-composed
// schema.
func CheckSchema(specDir string) []ValidationError {
	profile, perr := schema.ResolveProfile(specDir)
	if perr != nil {
		return []ValidationError{{
			Check:    "schema",
			Severity: "error",
			Path:     "profile.json",
			Message:  perr.Error(),
		}}
	}

	schemas, err := compileSchemas(profile)
	if err != nil {
		return []ValidationError{{
			Check:    "schema",
			Severity: "error",
			Path:     "",
			Message:  err.Error(),
		}}
	}

	var errs []ValidationError

	// Validate project.json.
	projPath := filepath.Join(specDir, "project.json")
	projErrs, projData := validateFile(projPath, "project.json", schemas.project)
	errs = append(errs, projErrs...)

	// If project.json failed to parse, we can't discover modules.
	if projData == nil {
		return errs
	}

	// Extract module paths from project.json to validate each module.json.
	modulePaths, extractErr := extractModulePaths(projData)
	if extractErr != nil {
		errs = append(errs, ValidationError{
			Check:    "schema",
			Severity: "error",
			Path:     "project.json",
			Message:  fmt.Sprintf("extract module paths: %s", extractErr),
		})
		return errs
	}

	for _, modPath := range modulePaths {
		modFilePath := filepath.Join(specDir, modPath, "module.json")
		displayPath := modPath + "/module.json"
		modErrs, _ := validateFile(modFilePath, displayPath, schemas.module)
		errs = append(errs, modErrs...)
	}

	return errs
}

// validateFile reads a JSON file, validates it against a compiled schema,
// and returns any violations plus the parsed JSON (nil if read/parse failed).
func validateFile(filePath, displayPath string, sch *jsonschema.Schema) ([]ValidationError, any) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return []ValidationError{{
			Check:    "schema",
			Severity: "error",
			Path:     displayPath,
			Message:  fmt.Sprintf("read file: %s", err),
		}}, nil
	}

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return []ValidationError{{
			Check:    "schema",
			Severity: "error",
			Path:     displayPath,
			Message:  fmt.Sprintf("invalid JSON: %s", err),
		}}, nil
	}

	err = sch.Validate(doc)
	if err == nil {
		return nil, doc
	}

	valErr, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []ValidationError{{
			Check:    "schema",
			Severity: "error",
			Path:     displayPath,
			Message:  err.Error(),
		}}, doc
	}

	return flattenValidationErrors(valErr, displayPath), doc
}

// flattenValidationErrors converts a jsonschema.ValidationError tree into
// a flat list of ValidationError values using BasicOutput.
func flattenValidationErrors(valErr *jsonschema.ValidationError, displayPath string) []ValidationError {
	output := valErr.BasicOutput()
	var errs []ValidationError
	for _, unit := range output.Errors {
		if unit.Error == nil {
			continue
		}
		msg := unit.Error.String()
		if msg == "" {
			continue
		}
		path := displayPath
		if unit.InstanceLocation != "" {
			path = displayPath + ":" + unit.InstanceLocation
		}
		errs = append(errs, ValidationError{
			Check:      "schema",
			Severity:   "error",
			Path:       path,
			Message:    msg,
			SchemaPath: unit.KeywordLocation,
		})
	}
	// If BasicOutput produced no leaf errors, fall back to the top-level error.
	if len(errs) == 0 {
		errs = append(errs, ValidationError{
			Check:    "schema",
			Severity: "error",
			Path:     displayPath,
			Message:  valErr.Error(),
		})
	}
	return errs
}

// extractModulePaths extracts module paths from parsed project.json data
// using type assertions (no marshal/unmarshal roundtrip needed).
func extractModulePaths(projData any) ([]string, error) {
	obj, ok := projData.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("project data is not an object")
	}
	modsRaw, ok := obj["modules"]
	if !ok {
		return nil, nil
	}
	mods, ok := modsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("modules field is not an array")
	}
	var paths []string
	for _, m := range mods {
		modObj, ok := m.(map[string]any)
		if !ok {
			continue
		}
		p, ok := modObj["path"].(string)
		if !ok {
			continue
		}
		paths = append(paths, strings.TrimRight(p, "/"))
	}
	return paths, nil
}
