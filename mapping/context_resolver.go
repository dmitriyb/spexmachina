package mapping

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
func ResolveContext(specDir string, record Record) (ContextResult, error) {
	compID, err := parseContextComponentID(record.SpecNodeID)
	if err != nil {
		return ContextResult{}, err
	}

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
		if containsInt(sec.Describes, compID) {
			implFiles = append(implFiles, filepath.Join(modDir, sec.Content))
		}
	}

	var testFiles []string
	for _, sec := range ms.TestSections {
		if containsInt(sec.Describes, compID) {
			testFiles = append(testFiles, filepath.Join(modDir, sec.Content))
		}
	}

	var flowFiles []string
	for _, df := range ms.DataFlows {
		if containsInt(df.Uses, compID) {
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

// parseContextComponentID extracts the integer component ID from a
// spec_node_id like "module_name/component/3". Returns an error if the
// format is invalid or the node type is not "component".
func parseContextComponentID(specNodeID string) (int, error) {
	parts := strings.Split(specNodeID, "/")
	if len(parts) != 3 {
		return 0, fmt.Errorf("context: invalid spec_node_id: %q (expected module/component/id)", specNodeID)
	}
	if parts[1] != "component" {
		return 0, fmt.Errorf("context: not a component node: %q", specNodeID)
	}
	id, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("context: invalid spec_node_id: %q (non-numeric id)", specNodeID)
	}
	return id, nil
}

// containsInt returns true if the slice contains the value.
func containsInt(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}
