package validator

import (
	"bytes"
	"encoding/json"
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

// compileSchemas loads and compiles the embedded JSON Schemas once.
func compileSchemas() (*compiledSchemas, error) {
	c := jsonschema.NewCompiler()

	projBytes, err := schema.ProjectSchema()
	if err != nil {
		return nil, fmt.Errorf("validator: load project schema: %w", err)
	}
	projDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(projBytes))
	if err != nil {
		return nil, fmt.Errorf("validator: parse project schema: %w", err)
	}
	if err := c.AddResource("project.schema.json", projDoc); err != nil {
		return nil, fmt.Errorf("validator: add project schema: %w", err)
	}

	modBytes, err := schema.ModuleSchema()
	if err != nil {
		return nil, fmt.Errorf("validator: load module schema: %w", err)
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
// against the embedded JSON Schemas. It returns all violations found.
func CheckSchema(specDir string) []ValidationError {
	schemas, err := compileSchemas()
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
	modules, extractErr := extractModulePaths(projData)
	if extractErr != nil {
		errs = append(errs, ValidationError{
			Check:    "schema",
			Severity: "error",
			Path:     "project.json",
			Message:  fmt.Sprintf("extract module paths: %s", extractErr),
		})
		return errs
	}

	for _, mod := range modules {
		modFilePath := filepath.Join(specDir, mod.path, "module.json")
		displayPath := mod.path + "/module.json"
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
			Check:    "schema",
			Severity: "error",
			Path:     path,
			Message:  msg,
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

// moduleInfo holds a module's path extracted from project.json.
type moduleInfo struct {
	path string
}

// extractModulePaths parses the project.json data to extract module paths.
func extractModulePaths(projData any) ([]moduleInfo, error) {
	// Re-marshal and unmarshal into the schema.Project type to get module paths.
	raw, err := json.Marshal(projData)
	if err != nil {
		return nil, fmt.Errorf("marshal project data: %w", err)
	}
	var proj schema.Project
	if err := json.Unmarshal(raw, &proj); err != nil {
		return nil, fmt.Errorf("unmarshal project: %w", err)
	}
	var modules []moduleInfo
	for _, m := range proj.Modules {
		path := strings.TrimRight(m.Path, "/")
		modules = append(modules, moduleInfo{path: path})
	}
	return modules, nil
}
