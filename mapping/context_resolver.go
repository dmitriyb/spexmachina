package mapping

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/dmitriyb/spexmachina/schema"
)

// ContextResult holds all resolved spec files for a component.
type ContextResult struct {
	Record     Record   `json:"record"`
	ArchFile   string   `json:"arch_file"`
	ImplFiles  []string `json:"impl_files"`
	TestFiles  []string `json:"test_files"`
	FlowFiles  []string `json:"flow_files"`
	ModuleFile string   `json:"module_file"`
}

// ResolveContext resolves all spec files needed to implement or review a
// component, given a mapping record and the spec directory. It is a pure
// function: deterministic, no side effects beyond reading files.
//
// record.SpecNodeID is treated as the component's identity hash directly —
// MappingStore's JSON-schema validation guarantees it matches ^[a-f0-9]{12}$,
// so no parsing happens here. record.Module is authoritative for locating
// module.json.
func ResolveContext(specDir string, record Record) (ContextResult, error) {
	compHash := record.SpecNodeID

	modPath := filepath.Join(specDir, record.Module, "module.json")
	data, err := os.ReadFile(modPath)
	if err != nil {
		return ContextResult{}, fmt.Errorf("context: read %s: %w", modPath, err)
	}

	var ms schema.ModuleSpec
	if err := json.Unmarshal(data, &ms); err != nil {
		return ContextResult{}, fmt.Errorf("context: parse %s: %w", modPath, err)
	}

	modDir := filepath.Join(specDir, record.Module)

	var implFiles []string
	for _, sec := range ms.ImplSections {
		if slices.Contains(sec.Describes, compHash) {
			implFiles = append(implFiles, filepath.Join(modDir, sec.Content))
		}
	}

	var testFiles []string
	for _, sec := range ms.TestSections {
		if slices.Contains(sec.Describes, compHash) {
			testFiles = append(testFiles, filepath.Join(modDir, sec.Content))
		}
	}

	var flowFiles []string
	for _, df := range ms.DataFlows {
		if slices.Contains(df.Uses, compHash) {
			flowFiles = append(flowFiles, filepath.Join(modDir, df.Content))
		}
	}

	return ContextResult{
		Record:     record,
		ArchFile:   record.ContentFile,
		ImplFiles:  implFiles,
		TestFiles:  testFiles,
		FlowFiles:  flowFiles,
		ModuleFile: modPath,
	}, nil
}
