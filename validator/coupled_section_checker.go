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

// CheckCoupledSections validates the project.json sections array. Every
// section must carry envelope fields (id, name, type) with unique ids and
// names. Sections with type "coupled" must also reference an existing module
// of the same name and validate against the module's section.schema.json.
// Sections of any other type are accepted with envelope-only checks.
func CheckCoupledSections(specDir string) []ValidationError {
	project, _, errs := loadSpec(specDir, "coupled_section")
	if len(errs) > 0 {
		return errs
	}
	if len(project.Sections) == 0 {
		return nil
	}

	modulePaths := make(map[string]string, len(project.Modules))
	moduleNames := make([]string, 0, len(project.Modules))
	for _, m := range project.Modules {
		modulePaths[m.Name] = m.Path
		moduleNames = append(moduleNames, m.Name)
	}

	var result []ValidationError
	seenIDs := make(map[int]int, len(project.Sections))
	seenNames := make(map[string]int, len(project.Sections))

	for i, section := range project.Sections {
		path := fmt.Sprintf("project.json:/sections/%d", i)
		envErrs, ok := checkSectionEnvelope(section, i, path)
		result = append(result, envErrs...)
		if !ok {
			continue
		}

		if prevIdx, dup := seenIDs[section.ID]; dup {
			result = append(result, ValidationError{
				Check:    "coupled_section",
				Severity: "error",
				Path:     path,
				Message:  fmt.Sprintf("duplicate section id %d (also at sections/%d)", section.ID, prevIdx),
			})
		} else {
			seenIDs[section.ID] = i
		}

		if prevIdx, dup := seenNames[section.Name]; dup {
			result = append(result, ValidationError{
				Check:    "coupled_section",
				Severity: "error",
				Path:     path,
				Message:  fmt.Sprintf("duplicate section name %q (also at sections/%d)", section.Name, prevIdx),
			})
		} else {
			seenNames[section.Name] = i
		}

		if section.Type != "coupled" {
			continue
		}

		result = append(result, checkCoupledSection(specDir, section, modulePaths, moduleNames, path)...)
	}

	return result
}

// checkSectionEnvelope verifies that id, name, and type are present in the
// section's raw JSON. Returns the envelope errors and a flag indicating
// whether downstream uniqueness/coupling checks should run for this section.
func checkSectionEnvelope(section schema.Section, index int, path string) ([]ValidationError, bool) {
	var errs []ValidationError
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(section.Raw, &raw); err != nil {
		errs = append(errs, ValidationError{
			Check:    "coupled_section",
			Severity: "error",
			Path:     path,
			Message:  fmt.Sprintf("section at index %d: parse JSON: %s", index, err),
		})
		return errs, false
	}

	missing := false
	for _, field := range []string{"id", "name", "type"} {
		if _, ok := raw[field]; !ok {
			errs = append(errs, ValidationError{
				Check:    "coupled_section",
				Severity: "error",
				Path:     path,
				Message:  fmt.Sprintf("section at index %d missing required field %q", index, field),
			})
			missing = true
		}
	}
	return errs, !missing
}

// checkCoupledSection validates a coupled section against its module:
// matching module exists, section.schema.json file exists, schema compiles,
// and the section content (envelope stripped) satisfies the schema.
func checkCoupledSection(specDir string, section schema.Section, modulePaths map[string]string, moduleNames []string, path string) []ValidationError {
	modPath, ok := modulePaths[section.Name]
	if !ok {
		msg := fmt.Sprintf("coupled section %q has no matching module; add a module with name %q", section.Name, section.Name)
		if near := nearestName(section.Name, moduleNames); near != "" {
			msg = fmt.Sprintf("coupled section %q has no matching module (did you mean %q?); add a module with name %q", section.Name, near, section.Name)
		}
		return []ValidationError{{
			Check:    "coupled_section",
			Severity: "error",
			Path:     path,
			Message:  msg,
		}}
	}

	schemaPath := filepath.Join(specDir, modPath, "section.schema.json")
	relSchemaPath := filepath.Join(modPath, "section.schema.json")
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []ValidationError{{
				Check:    "coupled_section",
				Severity: "error",
				Path:     path,
				Message:  fmt.Sprintf("coupled module %q is missing section.schema.json at %s", section.Name, relSchemaPath),
			}}
		}
		return []ValidationError{{
			Check:    "coupled_section",
			Severity: "error",
			Path:     path,
			Message:  fmt.Sprintf("read %s: %s", relSchemaPath, err),
		}}
	}

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		return []ValidationError{{
			Check:    "coupled_section",
			Severity: "error",
			Path:     path,
			Message:  fmt.Sprintf("parse %s as JSON: %s", relSchemaPath, err),
		}}
	}

	resourceURL := "section:" + relSchemaPath
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(resourceURL, doc); err != nil {
		return []ValidationError{{
			Check:    "coupled_section",
			Severity: "error",
			Path:     path,
			Message:  fmt.Sprintf("load %s: %s", relSchemaPath, err),
		}}
	}
	sectionSchema, err := compiler.Compile(resourceURL)
	if err != nil {
		return []ValidationError{{
			Check:    "coupled_section",
			Severity: "error",
			Path:     path,
			Message:  fmt.Sprintf("compile %s for module %q: %s", relSchemaPath, section.Name, err),
		}}
	}

	content, err := stripSectionEnvelope(section.Raw)
	if err != nil {
		return []ValidationError{{
			Check:    "coupled_section",
			Severity: "error",
			Path:     path,
			Message:  fmt.Sprintf("section %q: parse content: %s", section.Name, err),
		}}
	}

	if err := sectionSchema.Validate(content); err != nil {
		return contentValidationErrors(err, section.Name, relSchemaPath, path)
	}
	return nil
}

// stripSectionEnvelope re-parses the raw section JSON into a generic map and
// removes the envelope fields so the remainder can be validated against the
// module-supplied section schema.
func stripSectionEnvelope(raw json.RawMessage) (any, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		return doc, nil
	}
	delete(obj, "id")
	delete(obj, "name")
	delete(obj, "type")
	return obj, nil
}

// contentValidationErrors converts a JSON Schema validation failure into one or
// more ValidationError entries, including the failing instance path.
func contentValidationErrors(err error, sectionName, schemaPath, path string) []ValidationError {
	valErr, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []ValidationError{{
			Check:    "coupled_section",
			Severity: "error",
			Path:     path,
			Message:  fmt.Sprintf("section %q content failed validation against %s: %s", sectionName, schemaPath, err),
		}}
	}

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
		fieldPath := unit.InstanceLocation
		full := fmt.Sprintf("section %q content: %s", sectionName, msg)
		if fieldPath != "" {
			full = fmt.Sprintf("section %q content at %s: %s", sectionName, fieldPath, msg)
		}
		errs = append(errs, ValidationError{
			Check:      "coupled_section",
			Severity:   "error",
			Path:       path,
			Message:    full,
			SchemaPath: unit.KeywordLocation,
		})
	}
	if len(errs) == 0 {
		errs = append(errs, ValidationError{
			Check:    "coupled_section",
			Severity: "error",
			Path:     path,
			Message:  fmt.Sprintf("section %q content failed validation against %s: %s", sectionName, schemaPath, valErr),
		})
	}
	return errs
}

// nearestName returns the closest module name to target by case-insensitive
// equality. Empty string when no match is found.
func nearestName(target string, names []string) string {
	for _, n := range names {
		if strings.EqualFold(target, n) {
			return n
		}
	}
	return ""
}
