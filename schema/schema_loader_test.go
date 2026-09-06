package schema

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// --- IdentityHash tests (IH1-IH6) ---

func TestFR10_IH1_IdentityHashDeterministic(t *testing.T) {
	var results [10]string
	for i := range results {
		results[i] = IdentityHash("impact", "component", "NodeMatcher")
	}
	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Fatalf("call %d returned %q, call 0 returned %q", i, results[i], results[0])
		}
	}
}

func TestFR10_IH2_IdentityHashMatchesSchemaPattern(t *testing.T) {
	pat := regexp.MustCompile(`^[a-f0-9]{12}$`)
	inputs := [][]string{
		{"a"},
		{"a", "b"},
		{"a", "b", "c"},
		{"project", "requirement", "Define project schema"},
		{"module", "schema"},
		{"schema", "requirement", "Embed schemas in binary"},
		{"schema", "component", "ProjectSchema"},
		{"schema", "component", "ModuleSchema"},
		{"schema", "component", "SchemaLoader"},
		{"schema", "data_flow", "ValidateSpec"},
		{"schema", "test_section", "Schema validation tests"},
		{"validator", "component", "DAGChecker"},
		{"merkle", "component", "TreeBuilder"},
		{"impact", "component", "NodeMatcher"},
		{"map", "component", "MappingStore"},
		{"render", "component", "DOTRenderer"},
		{"x"},
		{"x", "y"},
		{"x", "y", "z"},
		{"long-module-name", "component", "LongComponentNameThatIsVeryDescriptive"},
		{"a", "b", "c", "d", "e"},
		{"unicode", "component", "Ünïcödë"},
		{"spaces", "component", "Has Spaces"},
		{"special", "chars!@#$%"},
		{"", "nonempty"},
		{"nonempty", ""},
	}
	for _, parts := range inputs {
		h := IdentityHash(parts...)
		if !pat.MatchString(h) {
			t.Errorf("IdentityHash(%v) = %q, does not match ^[a-f0-9]{12}$", parts, h)
		}
	}
}

func TestFR10_IH3_DifferentPartsProduceDifferentHashes(t *testing.T) {
	hashes := map[string]string{}
	inputs := [][]string{
		{"a", "b", "c"},
		{"a", "b", "d"},
		{"a", "c", "b"},
		{"d", "b", "c"},
	}
	for _, parts := range inputs {
		h := IdentityHash(parts...)
		key := strings.Join(parts, "/")
		for prevKey, prevHash := range hashes {
			if h == prevHash {
				t.Fatalf("collision: %q and %q both produce %q", key, prevKey, h)
			}
		}
		hashes[key] = h
	}
}

func TestFR10_IH4_SameNameDifferentModulesDiffer(t *testing.T) {
	h1 := IdentityHash("validator", "component", "Foo")
	h2 := IdentityHash("merkle", "component", "Foo")
	if h1 == h2 {
		t.Fatalf("expected different hashes for different modules, both got %q", h1)
	}
}

func TestFR10_IH5_JoinSeparatorIsSlash(t *testing.T) {
	h := IdentityHash("a", "b")
	sum := sha256.Sum256([]byte("a/b"))
	want := hex.EncodeToString(sum[:6])
	if h != want {
		t.Fatalf("IdentityHash(\"a\",\"b\") = %q, manual sha256(\"a/b\")[:6] = %q", h, want)
	}
}

// TestFR10_IH6_EmptyPartsAndEmptyInput asserts only totality and
// schema-valid output. It intentionally asserts no distinctness between the
// three calls: under join(parts, "/"), no parts and one empty part both
// yield the identity string "" and therefore the same hash — a collision
// SchemaLoader's leaf accepts as unreachable in practice, since every real
// part is a non-empty type literal, name, or title.
func TestFR10_IH6_EmptyPartsAndEmptyInput(t *testing.T) {
	pat := regexp.MustCompile(`^[a-f0-9]{12}$`)
	h1 := IdentityHash()
	h2 := IdentityHash("")
	h3 := IdentityHash("a", "", "b")

	for _, h := range []string{h1, h2, h3} {
		if !pat.MatchString(h) {
			t.Fatalf("degenerate input produced invalid hash %q", h)
		}
	}
}

// --- Schema loading integration tests (S1-S13, E1-E6, BM1-BM2) ---

func TestFR3_S1_ProjectSchemaLoads(t *testing.T) {
	data, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestFR3_S2_ModuleSchemaLoads(t *testing.T) {
	data, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestFR3_S3_ProjectSchemaStructure(t *testing.T) {
	data, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	checks := map[string]string{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://spexmachina.dev/schema/project.json",
		"title":   "Spex Machina Project",
		"type":    "object",
	}
	for key, want := range checks {
		got, _ := raw[key].(string)
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	required, _ := raw["required"].([]any)
	reqSet := map[string]bool{}
	for _, r := range required {
		reqSet[r.(string)] = true
	}
	for _, need := range []string{"name", "modules"} {
		if !reqSet[need] {
			t.Errorf("required missing %q", need)
		}
	}

	props, _ := raw["properties"].(map[string]any)
	for _, key := range []string{"name", "description", "version", "spec_version", "requirements", "modules", "sections"} {
		if props[key] == nil {
			t.Errorf("properties missing %q", key)
		}
	}

	// $defs is exhaustive on purpose: it is what catches a retired node
	// type (milestone, test_scenario) coming back through a stale embed.
	defs, _ := raw["$defs"].(map[string]any)
	wantDefs := map[string]bool{"identityHash": true, "requirement": true, "module": true, "section": true}
	for key := range wantDefs {
		if defs[key] == nil {
			t.Errorf("$defs missing %q", key)
		}
	}
	for key := range defs {
		if !wantDefs[key] {
			t.Errorf("$defs has unexpected key %q", key)
		}
	}

	if ap, ok := raw["additionalProperties"].(bool); !ok || ap {
		t.Error("additionalProperties should be false")
	}
}

func TestFR3_S4_ModuleSchemaStructure(t *testing.T) {
	data, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	checks := map[string]string{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://spexmachina.dev/schema/module.json",
		"title":   "Spex Machina Module",
		"type":    "object",
	}
	for key, want := range checks {
		got, _ := raw[key].(string)
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	required, _ := raw["required"].([]any)
	reqSet := map[string]bool{}
	for _, r := range required {
		reqSet[r.(string)] = true
	}
	if !reqSet["name"] {
		t.Error("required missing \"name\"")
	}

	props, _ := raw["properties"].(map[string]any)
	for _, key := range []string{"name", "description", "requirements", "components", "data_flows", "test_sections", "apis"} {
		if props[key] == nil {
			t.Errorf("properties missing %q", key)
		}
	}

	defs, _ := raw["$defs"].(map[string]any)
	for _, key := range []string{"requirement", "component", "data_flow", "test_section", "api"} {
		if defs[key] == nil {
			t.Errorf("$defs missing %q", key)
		}
	}

	if ap, ok := raw["additionalProperties"].(bool); !ok || ap {
		t.Error("additionalProperties should be false")
	}
}

func TestFR3_S5_BothSchemasIndependent(t *testing.T) {
	proj, err1 := ProjectSchema()
	mod, err2 := ModuleSchema()
	if err1 != nil {
		t.Fatalf("ProjectSchema(): %v", err1)
	}
	if err2 != nil {
		t.Fatalf("ModuleSchema(): %v", err2)
	}
	if bytes.Equal(proj, mod) {
		t.Fatal("project and module schemas are identical — expected different content")
	}

	var projRaw, modRaw map[string]any
	json.Unmarshal(proj, &projRaw)
	json.Unmarshal(mod, &modRaw)
	if projRaw["$id"] == modRaw["$id"] {
		t.Fatalf("$id values should differ, both are %v", projRaw["$id"])
	}
}

func compileSchema(t *testing.T, data []byte) *jsonschema.Schema {
	t.Helper()
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", raw); err != nil {
		t.Fatalf("add resource: %v", err)
	}
	sch, err := c.Compile("schema.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return sch
}

func validateJSON(t *testing.T, sch *jsonschema.Schema, doc []byte) error {
	t.Helper()
	var inst any
	if err := json.Unmarshal(doc, &inst); err != nil {
		t.Fatalf("unmarshal doc: %v", err)
	}
	return sch.Validate(inst)
}

func TestFR3_S6_ProjectSchemaValidatesFixture(t *testing.T) {
	schData, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	sch := compileSchema(t, schData)
	fixture := readTestdata(t, "valid_project.json")
	if err := validateJSON(t, sch, fixture); err != nil {
		t.Fatalf("valid_project.json should pass validation: %v", err)
	}
}

func TestFR3_S7_ModuleSchemaValidatesFixture(t *testing.T) {
	schData, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	sch := compileSchema(t, schData)
	fixture := readTestdata(t, "valid_module.json")
	if err := validateJSON(t, sch, fixture); err != nil {
		t.Fatalf("valid_module.json should pass validation: %v", err)
	}
}

func TestFR3_S8_ProjectSchemaRejectsInvalid(t *testing.T) {
	schData, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	sch := compileSchema(t, schData)
	// Missing required "modules"
	invalid := []byte(`{"name": "p"}`)
	if err := validateJSON(t, sch, invalid); err == nil {
		t.Fatal("expected validation error for missing modules, got nil")
	}
}

func TestFR3_S9_ModuleSchemaRejectsInvalid(t *testing.T) {
	schData, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	sch := compileSchema(t, schData)
	// Missing required "name"
	invalid := []byte(`{"components": [{"id": "aabbccddeeff", "name": "C"}]}`)
	if err := validateJSON(t, sch, invalid); err == nil {
		t.Fatal("expected validation error for missing name, got nil")
	}
}

// TestFR2_APISchemaRejectsInvalid pins the shape constraints of $defs/api:
// "required": ["id", "name"], "additionalProperties": false, and minLength on
// name. Without it the whole api definition can be weakened to
// "required": ["id"] plus "additionalProperties": true and no test notices.
func TestFR2_APISchemaRejectsInvalid(t *testing.T) {
	schData, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	sch := compileSchema(t, schData)

	tests := []struct {
		name    string
		doc     string
		wantErr bool
		wantRef string // substring the error must mention (empty = any error)
	}{
		{
			"full api passes",
			`{"name": "m", "apis": [{"id": "aabbccddeeff", "name": "spex diff", "description": "D", "provided_by": ["112233445566"], "group": "cli"}]}`,
			false, "",
		},
		{
			"minimal api passes",
			`{"name": "m", "apis": [{"id": "aabbccddeeff", "name": "spex diff"}]}`,
			false, "",
		},
		{
			"api missing name",
			`{"name": "m", "apis": [{"id": "aabbccddeeff"}]}`,
			true, "name",
		},
		{
			"api missing id",
			`{"name": "m", "apis": [{"name": "spex diff"}]}`,
			true, "id",
		},
		{
			"api with empty name",
			`{"name": "m", "apis": [{"id": "aabbccddeeff", "name": ""}]}`,
			true, "",
		},
		{
			"api with unknown property",
			`{"name": "m", "apis": [{"id": "aabbccddeeff", "name": "spex diff", "status": "done"}]}`,
			true, "",
		},
		{
			"api with duplicate provided_by items",
			`{"name": "m", "apis": [{"id": "aabbccddeeff", "name": "spex diff", "provided_by": ["112233445566", "112233445566"]}]}`,
			true, "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJSON(t, sch, []byte(tt.doc))
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("expected %s to validate, got: %v", tt.name, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.name)
			}
			if tt.wantRef != "" && !strings.Contains(err.Error(), tt.wantRef) {
				t.Fatalf("error should reference %q, got: %v", tt.wantRef, err)
			}
		})
	}
}

// TestNFR4_APIIDPatternValidation pins the ^[a-f0-9]{12}$ pattern on api.id
// and on every provided_by item, mirroring TestNFR4_IDPatternValidation for
// the other module node types.
func TestNFR4_APIIDPatternValidation(t *testing.T) {
	schData, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	sch := compileSchema(t, schData)

	tests := []struct {
		name    string
		doc     string
		wantErr bool
	}{
		{
			"valid 12-char hex id",
			`{"name": "m", "apis": [{"id": "aabbccddeeff", "name": "spex diff"}]}`,
			false,
		},
		{
			"too short id",
			`{"name": "m", "apis": [{"id": "aabbcc", "name": "spex diff"}]}`,
			true,
		},
		{
			"too long id",
			`{"name": "m", "apis": [{"id": "aabbccddeeff00", "name": "spex diff"}]}`,
			true,
		},
		{
			"uppercase hex id rejected",
			`{"name": "m", "apis": [{"id": "AABBCCDDEEFF", "name": "spex diff"}]}`,
			true,
		},
		{
			"non-hex characters rejected",
			`{"name": "m", "apis": [{"id": "aabbccddeegg", "name": "spex diff"}]}`,
			true,
		},
		{
			"empty id rejected",
			`{"name": "m", "apis": [{"id": "", "name": "spex diff"}]}`,
			true,
		},
		{
			"integer id rejected",
			`{"name": "m", "apis": [{"id": 1, "name": "spex diff"}]}`,
			true,
		},
		{
			"valid provided_by hashes",
			`{"name": "m", "apis": [{"id": "aabbccddeeff", "name": "spex diff", "provided_by": ["112233445566"]}]}`,
			false,
		},
		{
			"invalid provided_by item pattern",
			`{"name": "m", "apis": [{"id": "aabbccddeeff", "name": "spex diff", "provided_by": ["xyz"]}]}`,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJSON(t, sch, []byte(tt.doc))
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestFR3_S10_MinimalFixturesValidate(t *testing.T) {
	projData, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	modData, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	projSch := compileSchema(t, projData)
	modSch := compileSchema(t, modData)

	minProj := readTestdata(t, "minimal_project.json")
	if err := validateJSON(t, projSch, minProj); err != nil {
		t.Fatalf("minimal_project.json should pass: %v", err)
	}

	minMod := readTestdata(t, "minimal_module.json")
	if err := validateJSON(t, modSch, minMod); err != nil {
		t.Fatalf("minimal_module.json should pass: %v", err)
	}
}

func TestFR3_S11_ProjectSchemaIdempotent(t *testing.T) {
	d1, err1 := ProjectSchema()
	d2, err2 := ProjectSchema()
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v, %v", err1, err2)
	}
	if !bytes.Equal(d1, d2) {
		t.Fatal("ProjectSchema() returned different content on two calls")
	}
}

func TestFR3_S12_ModuleSchemaIdempotent(t *testing.T) {
	d1, err1 := ModuleSchema()
	d2, err2 := ModuleSchema()
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v, %v", err1, err2)
	}
	if !bytes.Equal(d1, d2) {
		t.Fatal("ModuleSchema() returned different content on two calls")
	}
}

func TestFR3_S13_GoTypesUnmarshalFromValidatedFixture(t *testing.T) {
	schData, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	sch := compileSchema(t, schData)
	fixture := readTestdata(t, "valid_module.json")

	if err := validateJSON(t, sch, fixture); err != nil {
		t.Fatalf("fixture should validate: %v", err)
	}

	var mod ModuleSpec
	if err := json.Unmarshal(fixture, &mod); err != nil {
		t.Fatalf("unmarshal into Go type: %v", err)
	}
	if mod.Name == "" {
		t.Fatal("mod.Name is empty after unmarshal")
	}
	if len(mod.Components) == 0 {
		t.Fatal("mod.Components is empty after unmarshal")
	}
	if len(mod.TestSections) == 0 {
		t.Fatal("mod.TestSections is empty after unmarshal")
	}
}

// --- Edge cases (E1-E6) ---

func TestFR3_E1_SchemasNonTrivialSize(t *testing.T) {
	proj, _ := ProjectSchema()
	mod, _ := ModuleSchema()
	if len(proj) <= 100 {
		t.Fatalf("ProjectSchema() too small: %d bytes", len(proj))
	}
	if len(mod) <= 100 {
		t.Fatalf("ModuleSchema() too small: %d bytes", len(mod))
	}
}

func TestFR3_E2_SchemaStartsWithJSONObject(t *testing.T) {
	for _, load := range []struct {
		name string
		fn   func() ([]byte, error)
	}{
		{"project", ProjectSchema},
		{"module", ModuleSchema},
		{"journal-line", JournalLineSchema},
	} {
		t.Run(load.name, func(t *testing.T) {
			data, err := load.fn()
			if err != nil {
				t.Fatalf("%s: %v", load.name, err)
			}
			trimmed := bytes.TrimLeft(data, " \t\n\r")
			if len(trimmed) == 0 || trimmed[0] != '{' {
				t.Fatalf("%s schema does not start with '{'", load.name)
			}
		})
	}
}

// TestFR3_E3_SchemaContentDeterministicAcrossBuilds compiles and runs a
// throwaway helper program that hashes ProjectSchema()'s output, twice from
// the same source, and checks the two builds print the same hash. This
// matters because the merkle module hashes schema files — a non-deterministic
// embed would break snapshot reproducibility.
func TestFR3_E3_SchemaContentDeterministicAcrossBuilds(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	// Built and run inside the repo tree (not t.TempDir(), which some
	// sandboxes mount noexec) so the built binary can actually execute, but
	// under .gotmp/ (repo-local, gitignored) rather than the repo root
	// itself: an untracked dir directly in the tracked tree flips
	// `go build`'s vcs.modified stamp for any OTHER build with cmd.Dir at
	// the repo root running concurrently (go test ./... parallelizes
	// packages), making that build non-reproducible. .gotmp/ is ignored, so
	// it never dirties the tree. See scripts/spec-gate_test.sh for the same
	// noexec-/tmp rationale and precedent.
	gotmp := filepath.Join(repoRoot, ".gotmp")
	if err := os.MkdirAll(gotmp, 0o755); err != nil {
		t.Fatalf("mkdir .gotmp: %v", err)
	}
	workDir, err := os.MkdirTemp(gotmp, "e3-build-")
	if err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(workDir) })

	src := `package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/dmitriyb/spexmachina/schema"
)

func main() {
	data, err := schema.ProjectSchema()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sum := sha256.Sum256(data)
	fmt.Print(hex.EncodeToString(sum[:]))
}
`
	mainPath := filepath.Join(workDir, "main.go")
	if err := os.WriteFile(mainPath, []byte(src), 0o644); err != nil {
		t.Fatalf("write helper source: %v", err)
	}

	build := func(n int) string {
		t.Helper()
		outPath := filepath.Join(workDir, fmt.Sprintf("schemahash%d", n))
		cmd := exec.Command("go", "build", "-o", outPath, mainPath)
		// Each build gets its own GOCACHE. Sharing the ambient one would let
		// the second build serve as a cache hit for the first, so the two
		// runs would never actually re-execute the embed and the comparison
		// below could never fail. See delivery/release_build_test.go's
		// buildSpex for the same hazard and fix.
		cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(workDir, fmt.Sprintf("cache%d", n)))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go build #%d: %v\n%s", n, err, out)
		}
		out, err := exec.Command(outPath).Output()
		if err != nil {
			t.Fatalf("run build #%d: %v", n, err)
		}
		return string(out)
	}

	hash1 := build(1)
	hash2 := build(2)
	if hash1 == "" {
		t.Fatal("empty hash from build #1")
	}
	if hash1 != hash2 {
		t.Fatalf("build #1 hash %q differs from build #2 hash %q", hash1, hash2)
	}
}

func TestFR3_E4_ConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	results := make([][]byte, 20)
	errs := make([]error, 20)
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = ProjectSchema()
		}(i * 2)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = ModuleSchema()
		}(i*2 + 1)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d error: %v", i, err)
		}
	}
	// All project results identical
	for i := 2; i < 20; i += 2 {
		if !bytes.Equal(results[0], results[i]) {
			t.Fatalf("project schema differs between goroutines 0 and %d", i)
		}
	}
	// All module results identical
	for i := 3; i < 20; i += 2 {
		if !bytes.Equal(results[1], results[i]) {
			t.Fatalf("module schema differs between goroutines 1 and %d", i)
		}
	}
}

func TestFR3_E5_NonExistentSchemaFile(t *testing.T) {
	_, err := schemaFS.ReadFile("nonexistent.schema.json")
	if err == nil {
		t.Fatal("expected error reading nonexistent schema file, got nil")
	}
}

func TestFR3_E6_SchemaRefsResolveToDefs(t *testing.T) {
	data, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)

	defs, _ := raw["$defs"].(map[string]any)
	props, _ := raw["properties"].(map[string]any)

	// Check that components.items.$ref resolves to a $defs entry
	comps, _ := props["components"].(map[string]any)
	items, _ := comps["items"].(map[string]any)
	ref, _ := items["$ref"].(string)
	if ref != "#/$defs/component" {
		t.Fatalf("components.$ref = %q, want #/$defs/component", ref)
	}
	if defs["component"] == nil {
		t.Fatal("$defs/component missing")
	}
}

// --- JournalLineSchema / TaskStateSchema loading tests (JL1-JL2, TS1-TS2) ---

func TestFR3_JL1_JournalLineSchemaLoads(t *testing.T) {
	data, err := JournalLineSchema()
	if err != nil {
		t.Fatalf("JournalLineSchema(): %v", err)
	}
	if len(data) == 0 {
		t.Fatal("JournalLineSchema() returned empty bytes")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

// TestFR3_JL2_JournalLineSchemaAcceptsBothIdentities checks that node keys
// and proposal slugs live in different fields with different constraints —
// the identity-hash pattern lives where the hash lives, and slugs never
// share its field.
func TestFR3_JL2_JournalLineSchemaAcceptsBothIdentities(t *testing.T) {
	data, err := JournalLineSchema()
	if err != nil {
		t.Fatalf("JournalLineSchema(): %v", err)
	}
	sch := compileSchema(t, data)

	// A change event's node is the identity hash.
	changeEvent := `{"event":"added","eid":"9f2c41a0b7d3","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`
	if err := validateJSON(t, sch, []byte(changeEvent)); err != nil {
		t.Fatalf("change event with identity-hash node should pass: %v", err)
	}

	// An epic receipt's proposal is the slug-shaped reference.
	epicReceipt := `{"event":"task_created","proposal":"2026-04-12-data-flow-contract-layer","task_id":"spexmachina-epic"}`
	if err := validateJSON(t, sch, []byte(epicReceipt)); err != nil {
		t.Fatalf("epic receipt with proposal slug should pass: %v", err)
	}

	// An empty node always fails the identity-hash pattern.
	emptyNode := `{"event":"added","eid":"9f2c41a0b7d3","node":"","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`
	if err := validateJSON(t, sch, []byte(emptyNode)); err == nil {
		t.Fatal("empty node should fail the identity-hash pattern")
	}
}

func TestFR3_TS1_TaskStateSchemaLoads(t *testing.T) {
	data, err := TaskStateSchema()
	if err != nil {
		t.Fatalf("TaskStateSchema(): %v", err)
	}
	if len(data) == 0 {
		t.Fatal("TaskStateSchema() returned empty bytes")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	// Must also compile as a JSON Schema document, not merely parse as JSON.
	compileSchema(t, data)
}

// TestFR3_TS2_EmbeddedTaskStateSchemaIsWhatPlanValidatesAgainst pins that the
// loader serves the same document TaskReader refuses a "closed" status
// against, so the two cannot drift: one embedded file, read by one function,
// compiled by its one consumer.
func TestFR3_TS2_EmbeddedTaskStateSchemaIsWhatPlanValidatesAgainst(t *testing.T) {
	data, err := TaskStateSchema()
	if err != nil {
		t.Fatalf("TaskStateSchema(): %v", err)
	}
	sch := compileSchema(t, data)

	open := `{"version": 1, "tasks": [{"task_id": "spexmachina-abc", "status": "open"}]}`
	if err := validateJSON(t, sch, []byte(open)); err != nil {
		t.Fatalf("open status should pass: %v", err)
	}

	closed := `{"version": 1, "tasks": [{"task_id": "spexmachina-abc", "status": "closed"}]}`
	err = validateJSON(t, sch, []byte(closed))
	if err == nil {
		t.Fatal("closed status should fail the status enum")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Fatalf("error should name the status constraint, got: %v", err)
	}
}

// --- ProfileLoader tests (P1-P5) ---

// jsonEqual compares two JSON documents structurally, ignoring key order —
// the composed schemas re-marshal from map[string]any, whose keys
// encoding/json always emits sorted, so a byte-for-byte comparison against
// the hand-authored shipped documents would fail on ordering alone even
// when the content is identical.
func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var va, vb any
	if err := json.Unmarshal(a, &va); err != nil {
		t.Fatalf("unmarshal a: %v", err)
	}
	if err := json.Unmarshal(b, &vb); err != nil {
		t.Fatalf("unmarshal b: %v", err)
	}
	return reflect.DeepEqual(va, vb)
}

func TestFR9_P1_AbsentProfileResolvesToDefault(t *testing.T) {
	dir := t.TempDir()

	p, err := ResolveProfile(dir)
	if err != nil {
		t.Fatalf("ResolveProfile over spec dir with no profile.json: %v", err)
	}

	if !reflect.DeepEqual(p, DefaultProfile()) {
		t.Fatalf("resolved profile over an absent profile.json should equal DefaultProfile()")
	}

	names := map[string]bool{}
	completenessTrigger := map[string]bool{}
	nameDeclarable := map[string]bool{}
	referenceFieldsByType := map[string]map[string]Field{}
	for _, nt := range p.NodeTypes {
		names[nt.Name] = true
		if nt.CompletenessTrigger {
			completenessTrigger[nt.Name] = true
		}
		if nt.NameDeclarable {
			nameDeclarable[nt.Name] = true
		}
		fields := map[string]Field{}
		for _, f := range nt.Fields {
			if f.Kind == FieldKindReference {
				fields[f.Name] = f
				if f.Cyclic {
					t.Fatalf("field %q on %q should not carry the cyclic exemption under the default profile", f.Name, nt.Name)
				}
			}
		}
		referenceFieldsByType[nt.Scope+":"+nt.Name] = fields
	}
	wantNames := []string{"requirement", "component", "data_flow", "test_section", "api"}
	if len(names) != len(wantNames) {
		t.Fatalf("node type names = %v, want exactly %v", names, wantNames)
	}
	for _, n := range wantNames {
		if !names[n] {
			t.Fatalf("node types missing %q: %v", n, names)
		}
	}
	if !completenessTrigger["requirement"] || len(completenessTrigger) != 1 {
		t.Fatalf("completeness trigger should be marked on exactly requirement, got %v", completenessTrigger)
	}
	if len(nameDeclarable) != 2 || !nameDeclarable["component"] || !nameDeclarable["api"] {
		t.Fatalf("name-declarable role should be marked on exactly component and api, got %v", nameDeclarable)
	}

	// Every reference kind the earlier edges section carried is now a field
	// declared on its source type.
	wantReferenceFields := map[string][]string{
		"module:requirement":  {"preq_id", "depends_on"},
		"project:requirement": {"depends_on"},
		"module:component":    {"implements", "uses"},
		"module:data_flow":    {"uses"},
		"module:test_section": {"describes"},
		"module:api":          {"provided_by"},
	}
	for typeKey, fieldNames := range wantReferenceFields {
		got := referenceFieldsByType[typeKey]
		if len(got) != len(fieldNames) {
			t.Fatalf("%s: reference fields = %v, want %v", typeKey, got, fieldNames)
		}
		for _, fn := range fieldNames {
			if _, ok := got[fn]; !ok {
				t.Fatalf("%s: missing reference field %q", typeKey, fn)
			}
		}
	}

	if len(p.Edges) != 7 {
		t.Fatalf("derived edges = %d, want 7", len(p.Edges))
	}
	wantEdgeKinds := map[string]bool{
		"preq_id": true, "implements": true, "uses": true, "provided_by": true,
		"describes": true, "depends_on": true, "requires_module": true,
	}
	for _, e := range p.Edges {
		if !wantEdgeKinds[e.Kind] {
			t.Fatalf("unexpected derived edge kind %q", e.Kind)
		}
		for _, from := range e.From {
			if e.CyclicForType(from) {
				t.Fatalf("derived edge %q from %q should not carry the cyclic exemption under the default profile", e.Kind, from)
			}
		}
	}

	if len(p.CoverageChains) != 3 {
		t.Fatalf("coverage chains = %d, want 3", len(p.CoverageChains))
	}

	wantPlanRelevant := []string{"data_flow", "component", "test_section"}
	if !reflect.DeepEqual(p.PlanRelevant, wantPlanRelevant) {
		t.Fatalf("plan-relevant list = %v, want %v in declared order", p.PlanRelevant, wantPlanRelevant)
	}

	wantImpact := map[string]string{
		"test_section": "impl_only",
		"data_flow":    "contract",
		"api":          "contract",
		"component":    "arch_impl",
		"requirement":  "structural",
	}
	if !reflect.DeepEqual(p.ImpactLevels, wantImpact) {
		t.Fatalf("impact levels = %v, want %v", p.ImpactLevels, wantImpact)
	}

	wantHashedFields := map[string][]string{
		"project:requirement": {"depends_on", "description", "id", "name", "priority", "type"},
		"module:requirement":  {"depends_on", "description", "id", "name", "preq_id", "type"},
		"module:api":          {"description", "group", "id", "name", "provided_by"},
	}
	if !reflect.DeepEqual(p.HashedFields, wantHashedFields) {
		t.Fatalf("hashed field allowlists = %v, want %v", p.HashedFields, wantHashedFields)
	}

	// Every declared node type is absorbable in both directions: refresh
	// is the authoring loop's deliberate override, taken with a stated
	// reason, so the default profile does not withhold any of its own
	// types from it. "meta" and "module" are the frame's fixed leaves and
	// appear in no profile's table.
	wantAbsorbable := map[string]AbsorbDirections{
		"requirement":  {Added: true, Removed: true},
		"api":          {Added: true, Removed: true},
		"component":    {Added: true, Removed: true},
		"data_flow":    {Added: true, Removed: true},
		"test_section": {Added: true, Removed: true},
	}
	if !reflect.DeepEqual(p.Absorbable, wantAbsorbable) {
		t.Fatalf("absorbable directions = %v, want %v", p.Absorbable, wantAbsorbable)
	}
}

// TestFR9_DeriveEdgesCyclicIsPerDeclaringType covers 91270a8a2b57's per-field
// exemption flag: two node types declaring a reference field of the same
// name must keep their own Cyclic value independently. Before this fix,
// deriveEdges took Cyclic from whichever type happened to declare the field
// first and every later declaration was silently ignored, so declaration
// order — not the profile's own text — decided which type's fields got
// cycle-checked.
func TestFR9_DeriveEdgesCyclicIsPerDeclaringType(t *testing.T) {
	build := func(aCyclic, bCyclic bool) []Edge {
		nodeTypes := []NodeType{
			{Name: "a", PluralKey: "as", Scope: "module", Fields: []Field{
				{Name: "uses", Kind: FieldKindReference, Targets: []string{"a"}, Cyclic: aCyclic},
			}},
			{Name: "b", PluralKey: "bs", Scope: "module", Fields: []Field{
				{Name: "uses", Kind: FieldKindReference, Targets: []string{"a"}, Cyclic: bCyclic},
			}},
		}
		return deriveEdges(nodeTypes)
	}

	check := func(t *testing.T, edges []Edge, wantA, wantB bool) {
		t.Helper()
		var uses *Edge
		for i, e := range edges {
			if e.Kind == "uses" {
				uses = &edges[i]
			}
		}
		if uses == nil {
			t.Fatal("no derived \"uses\" edge")
		}
		if got := uses.CyclicForType("a"); got != wantA {
			t.Fatalf("a's cyclic exemption = %v, want %v", got, wantA)
		}
		if got := uses.CyclicForType("b"); got != wantB {
			t.Fatalf("b's cyclic exemption = %v, want %v", got, wantB)
		}
	}

	t.Run("a declared first, b marks cyclic", func(t *testing.T) {
		check(t, build(false, true), false, true)
	})
	t.Run("b declared first (reverse order), a marks cyclic", func(t *testing.T) {
		nodeTypes := []NodeType{
			{Name: "b", PluralKey: "bs", Scope: "module", Fields: []Field{
				{Name: "uses", Kind: FieldKindReference, Targets: []string{"a"}, Cyclic: false},
			}},
			{Name: "a", PluralKey: "as", Scope: "module", Fields: []Field{
				{Name: "uses", Kind: FieldKindReference, Targets: []string{"a"}, Cyclic: true},
			}},
		}
		check(t, deriveEdges(nodeTypes), true, false)
	})
}

// TestFR9_P2_ComposedSchemasEqualShippedGolden is the acceptance criterion
// from arch_profile_loader.md's "The default profile is the golden policy
// record": composing project and module schemas from the profile resolved
// over an absent profile.json must reproduce the shipped static schema
// documents exactly.
func TestFR9_P2_ComposedSchemasEqualShippedGolden(t *testing.T) {
	p, err := ResolveProfile(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}

	composedProject, err := ComposeProjectSchema(p.ProjectNodeTypes(), p.Edges)
	if err != nil {
		t.Fatalf("ComposeProjectSchema: %v", err)
	}
	shippedProject, err := projectSchemaBytes()
	if err != nil {
		t.Fatalf("projectSchemaBytes: %v", err)
	}
	if !jsonEqual(t, composedProject, shippedProject) {
		t.Fatalf("project schema composed from the default profile does not reproduce the shipped document")
	}

	composedModule, err := ComposeModuleSchema(p.ModuleNodeTypes(), p.Edges)
	if err != nil {
		t.Fatalf("ComposeModuleSchema: %v", err)
	}
	shippedModule, err := moduleSchemaBytes()
	if err != nil {
		t.Fatalf("moduleSchemaBytes: %v", err)
	}
	if !jsonEqual(t, composedModule, shippedModule) {
		t.Fatalf("module schema composed from the default profile does not reproduce the shipped document")
	}
}

func TestFR9_P3_MalformedProfileIsDistinctEarlyFailure(t *testing.T) {
	t.Run("unparseable JSON", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte("{invalid json"), 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		_, err := ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error resolving an unparseable profile.json, got nil")
		}
		if !strings.Contains(err.Error(), "profile.json") {
			t.Fatalf("error should name the profile file, got: %v", err)
		}
	})

	t.Run("node type with no plural array key", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{
			"node_types": [
				{"name": "endpoint", "scope": "module"}
			]
		}`
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(doc), 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		_, err := ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error resolving a profile with a node type missing plural_key, got nil")
		}
		if !strings.Contains(err.Error(), "profile.json") {
			t.Fatalf("error should name the profile file, got: %v", err)
		}
		if !strings.Contains(err.Error(), "plural_key") {
			t.Fatalf("error should name the defect (plural_key), got: %v", err)
		}
	})

	t.Run("retired edges/hashed_fields format is rejected as malformed", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{
			"node_types": [
				{"name": "widget", "plural_key": "widgets", "scope": "module"}
			],
			"edges": [],
			"hashed_fields": {}
		}`
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(doc), 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		_, err := ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error resolving a pre-versioning edges/hashed_fields document, got nil")
		}
		if !strings.Contains(err.Error(), "profile.json") {
			t.Fatalf("error should name the profile file, got: %v", err)
		}
		if !strings.Contains(err.Error(), "edges") {
			t.Fatalf("error should name the unrecognized retired-format key (edges), got: %v", err)
		}
	})
}

// TestFR9_P4_DeclaredCustomTypeReachesComposedSchema declares a custom
// "endpoint" type in spec/profile.json — module-scoped, plural key
// "endpoints", content leaf required, with a text field carrying an
// enumeration and a reference field targeting "component" — and checks that
// it reaches the composed module schema through the file-backed
// ResolveProfile -> ComposeModuleSchema path with the same generic envelope
// constraints built-in types get, one composed property per declared field,
// and that additionalProperties:false rejects an undeclared array, an
// undeclared field, and a non-hash id, while the required content leaf is
// enforced.
func TestFR9_P4_DeclaredCustomTypeReachesComposedSchema(t *testing.T) {
	profile := DefaultProfile()
	profile.NodeTypes = append(profile.NodeTypes, NodeType{
		Name:            "endpoint",
		PluralKey:       "endpoints",
		Scope:           "module",
		RequiresContent: true,
		Fields: []Field{
			{Name: "protocol", Kind: FieldKindText, Enum: []string{"http", "grpc"}},
			{Name: "serves", Kind: FieldKindReference, Targets: []string{"component"}},
		},
	})

	dir := t.TempDir()
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.json"), data, 0o644); err != nil {
		t.Fatalf("write profile.json: %v", err)
	}

	resolved, err := ResolveProfile(dir)
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}

	composed, err := ComposeModuleSchema(resolved.ModuleNodeTypes(), resolved.Edges)
	if err != nil {
		t.Fatalf("ComposeModuleSchema: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(composed, &raw); err != nil {
		t.Fatalf("unmarshal composed schema: %v", err)
	}
	def, ok := raw["$defs"].(map[string]any)["endpoint"].(map[string]any)
	if !ok {
		t.Fatal("$defs/endpoint missing from composed module schema")
	}
	props, ok := def["properties"].(map[string]any)
	if !ok {
		t.Fatal("$defs/endpoint has no properties")
	}

	protocol, ok := props["protocol"].(map[string]any)
	if !ok {
		t.Fatalf("$defs/endpoint should carry the declared 'protocol' field, got properties: %v", props)
	}
	if enum, ok := protocol["enum"].([]any); !ok || len(enum) != 2 || enum[0] != "http" || enum[1] != "grpc" {
		t.Fatalf("protocol should compose enum-constrained [http grpc], got: %v", protocol)
	}

	serves, ok := props["serves"].(map[string]any)
	if !ok {
		t.Fatalf("$defs/endpoint should carry the declared 'serves' field, got properties: %v", props)
	}
	if serves["type"] != "array" {
		t.Fatalf("serves should compose as an array of identity hashes, got: %v", serves)
	}
	items, ok := serves["items"].(map[string]any)
	if !ok || items["pattern"] != identityHashPattern {
		t.Fatalf("serves items should be identity-hash-pattern strings, got: %v", serves)
	}

	sch := compileSchemaFromBytes(t, composed)

	if err := validateModule(t, sch, `{
		"name": "m",
		"endpoints": [
			{"id": "aabbccddeeff", "name": "GET /things", "content": "endpoint_things.md", "protocol": "http", "serves": ["112233445566"]}
		]
	}`); err != nil {
		t.Fatalf("fully-populated endpoint entry should pass: %v", err)
	}

	if err := validateModule(t, sch, `{
		"name": "m",
		"endpoints": [
			{"id": "aabbccddeeff", "name": "GET /things", "content": "endpoint_things.md", "protocol": "ftp"}
		]
	}`); err == nil {
		t.Fatal("expected validation error for a protocol value outside the declared enum")
	}

	if err := validateModule(t, sch, `{
		"name": "m",
		"endpoints": [
			{"id": "aabbccddeeff", "name": "GET /things", "protocol": "http"}
		]
	}`); err == nil {
		t.Fatal("expected validation error for an endpoint entry omitting its required content leaf")
	}

	if err := validateModule(t, sch, `{
		"name": "m",
		"endpoints": [
			{"id": "not-a-hash", "name": "GET /things", "content": "endpoint_things.md"}
		]
	}`); err == nil {
		t.Fatal("expected validation error for a non-hash id")
	}

	if err := validateModule(t, sch, `{
		"name": "m",
		"endpoints": [
			{"id": "aabbccddeeff", "name": "GET /things", "content": "endpoint_things.md", "unlisted_field": "x"}
		]
	}`); err == nil {
		t.Fatal("expected validation error for an undeclared field on the endpoint entry")
	}

	if err := validateModule(t, sch, `{
		"name": "m",
		"widgets": []
	}`); err == nil {
		t.Fatal("expected validation error for an array the profile does not declare")
	}
}

func TestFR9_P5_ResolutionIsDeterministic(t *testing.T) {
	t.Run("default profile", func(t *testing.T) {
		dir := t.TempDir()
		p1, err := ResolveProfile(dir)
		if err != nil {
			t.Fatalf("ResolveProfile #1: %v", err)
		}
		p2, err := ResolveProfile(dir)
		if err != nil {
			t.Fatalf("ResolveProfile #2: %v", err)
		}
		if !reflect.DeepEqual(p1, p2) {
			t.Fatal("resolving the default profile twice should be byte-identical")
		}

		c1, err := ComposeModuleSchema(p1.ModuleNodeTypes(), p1.Edges)
		if err != nil {
			t.Fatalf("ComposeModuleSchema #1: %v", err)
		}
		c2, err := ComposeModuleSchema(p2.ModuleNodeTypes(), p2.Edges)
		if err != nil {
			t.Fatalf("ComposeModuleSchema #2: %v", err)
		}
		if !bytes.Equal(c1, c2) {
			t.Fatal("composing the module schema from the default profile twice should be byte-identical")
		}
	})

	t.Run("file-backed profile", func(t *testing.T) {
		dir := t.TempDir()
		data, err := json.Marshal(DefaultProfile())
		if err != nil {
			t.Fatalf("marshal profile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), data, 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}

		p1, err := ResolveProfile(dir)
		if err != nil {
			t.Fatalf("ResolveProfile #1: %v", err)
		}
		p2, err := ResolveProfile(dir)
		if err != nil {
			t.Fatalf("ResolveProfile #2: %v", err)
		}
		if !reflect.DeepEqual(p1, p2) {
			t.Fatal("resolving the same file-backed profile twice should be byte-identical")
		}
	})
}

// TestFR9_P6_NewReferenceFieldOnBuiltinTypeComposes covers test_schema_loading.md's
// P6: a profile-declared reference field on a built-in type — one the
// shipped frame never carried — reaches the composed module schema beside
// the built-in fields. This supersedes the earlier interim rule that
// refused a new edge kind sourced at a built-in type: built-in $defs are no
// longer frame-fixed against a field the frame does not already give them.
func TestFR9_P6_NewReferenceFieldOnBuiltinTypeComposes(t *testing.T) {
	profile := DefaultProfile()
	profile.NodeTypes = append(profile.NodeTypes, NodeType{
		Name:            "endpoint",
		PluralKey:       "endpoints",
		Scope:           "module",
		RequiresContent: true,
	})
	for i, nt := range profile.NodeTypes {
		if nt.Scope == "module" && nt.Name == "component" {
			profile.NodeTypes[i].Fields = append(profile.NodeTypes[i].Fields, Field{
				Name: "audits", Kind: FieldKindReference, Targets: []string{"endpoint"},
			})
		}
	}

	dir := t.TempDir()
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.json"), data, 0o644); err != nil {
		t.Fatalf("write profile.json: %v", err)
	}

	resolved, err := ResolveProfile(dir)
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}

	composed, err := ComposeModuleSchema(resolved.ModuleNodeTypes(), resolved.Edges)
	if err != nil {
		t.Fatalf("ComposeModuleSchema: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(composed, &raw); err != nil {
		t.Fatalf("unmarshal composed schema: %v", err)
	}
	def, ok := raw["$defs"].(map[string]any)["component"].(map[string]any)
	if !ok {
		t.Fatal("$defs/component missing from composed module schema")
	}
	props, ok := def["properties"].(map[string]any)
	if !ok {
		t.Fatal("$defs/component has no properties")
	}
	audits, ok := props["audits"].(map[string]any)
	if !ok {
		t.Fatalf("$defs/component should carry the profile-declared 'audits' field beside the built-in fields, got properties: %v", props)
	}
	if audits["type"] != "array" {
		t.Fatalf("audits should compose as an array-of-identity-hash property, got: %v", audits)
	}
	if _, ok := props["implements"]; !ok {
		t.Fatal("built-in 'implements' field should still be present beside the new one")
	}
}

// TestFR9_P7_FieldValidationNamesEachDefect covers test_schema_loading.md's
// P7: each of the six defective field-declaration shapes fails with one
// distinct, early error naming the declaration, with no composed schema
// produced.
func TestFR9_P7_FieldValidationNamesEachDefect(t *testing.T) {
	base := func(field map[string]any) string {
		doc := map[string]any{
			"node_types": []any{
				map[string]any{
					"name": "widget", "plural_key": "widgets", "scope": "module",
					"fields": []any{field},
				},
			},
		}
		data, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("marshal profile: %v", err)
		}
		return string(data)
	}

	cases := []struct {
		name    string
		field   map[string]any
		wantErr string
	}{
		{
			name:    "unknown kind",
			field:   map[string]any{"name": "flavor", "kind": "enum"},
			wantErr: "unknown kind",
		},
		{
			name:    "reference field naming an undeclared target type",
			field:   map[string]any{"name": "refers", "kind": "reference", "targets": []string{"gadget"}},
			wantErr: "undeclared node type",
		},
		{
			name:    "enumeration on a non-text field",
			field:   map[string]any{"name": "count", "kind": "integer", "enum": []string{"a", "b"}},
			wantErr: "enum is only valid on a text field",
		},
		{
			name:    "bounds on a non-integer field",
			field:   map[string]any{"name": "flavor", "kind": "text", "minimum": 0},
			wantErr: "minimum/maximum are only valid on an integer field",
		},
		{
			name:    "field name colliding with an envelope field",
			field:   map[string]any{"name": "id", "kind": "text"},
			wantErr: "collides with the envelope",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(base(tc.field)), 0o644); err != nil {
				t.Fatalf("write profile.json: %v", err)
			}
			_, err := ResolveProfile(dir)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "profile.json") {
				t.Fatalf("error should name the profile file, got: %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error should contain %q, got: %v", tc.wantErr, err)
			}
		})
	}

	t.Run("node type named module", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{
			"node_types": [
				{"name": "module", "plural_key": "modules", "scope": "project", "fields": [
					{"name": "requires_module", "kind": "reference", "targets": ["module"], "cyclic": true}
				]}
			]
		}`
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(doc), 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		_, err := ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error, got nil: module is the frame's fixed type and must not be declarable")
		}
		if !strings.Contains(err.Error(), "profile.json") {
			t.Fatalf("error should name the profile file, got: %v", err)
		}
		if !strings.Contains(err.Error(), `"module" is the frame's fixed type`) {
			t.Fatalf("error should name the fixed-point declaration, got: %v", err)
		}
	})

	t.Run("duplicate field name within one type", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{
			"node_types": [
				{"name": "widget", "plural_key": "widgets", "scope": "module", "fields": [
					{"name": "flavor", "kind": "text"},
					{"name": "flavor", "kind": "text"}
				]}
			]
		}`
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(doc), 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		_, err := ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "duplicate field name") {
			t.Fatalf("error should name the duplicate field defect, got: %v", err)
		}
	})
}

// TestFR9_ContentFieldCollisionIsConditional covers arch_profile_loader.md's
// "Fixed points": content is part of the fixed envelope only for a
// content-bearing type ("content, where the type is content-bearing"), so a
// declared field named "content" collides with the envelope only when the
// declaring type sets RequiresContent — not unconditionally, the way id,
// name and description always do.
func TestFR9_ContentFieldCollisionIsConditional(t *testing.T) {
	t.Run("content-bearing type rejects a declared content field", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{
			"node_types": [
				{"name": "widget", "plural_key": "widgets", "scope": "module", "requires_content": true, "fields": [
					{"name": "content", "kind": "text"}
				]}
			]
		}`
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(doc), 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		_, err := ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error for a content-bearing type declaring a \"content\" field, got nil")
		}
		if !strings.Contains(err.Error(), "collides with the envelope") {
			t.Fatalf("error should say the field collides with the envelope, got: %v", err)
		}
	})

	t.Run("non-content-bearing type may declare a content field", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{
			"node_types": [
				{"name": "widget", "plural_key": "widgets", "scope": "module", "fields": [
					{"name": "content", "kind": "text"}
				]}
			]
		}`
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(doc), 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		if _, err := ResolveProfile(dir); err != nil {
			t.Fatalf("a non-content-bearing type declaring a \"content\" field should resolve cleanly: %v", err)
		}
	})
}

// TestFR9_P8_ProfileVersionOutOfRangeFailsEarly covers test_schema_loading.md's
// P8: an out-of-range profile_version fails with one message naming the
// version and the supported range, before any conformance check; an absent
// profile_version means version 1, which resolves cleanly.
func TestFR9_P8_ProfileVersionOutOfRangeFailsEarly(t *testing.T) {
	t.Run("unsupported version fails early", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{"profile_version": 99, "node_types": []}`
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(doc), 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		_, err := ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error resolving an out-of-range profile_version, got nil")
		}
		if !strings.Contains(err.Error(), "profile.json") {
			t.Fatalf("error should name the profile file, got: %v", err)
		}
		if !strings.Contains(err.Error(), "99") {
			t.Fatalf("error should name the declared version (99), got: %v", err)
		}
	})

	t.Run("absent profile_version means version 1", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{"node_types": []}`
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(doc), 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		p, err := ResolveProfile(dir)
		if err != nil {
			t.Fatalf("expected an absent profile_version to resolve as version 1, got: %v", err)
		}
		if p.ProfileVersion != nil && *p.ProfileVersion != 1 {
			t.Fatalf("resolved ProfileVersion = %d, want nil (absent) or 1", *p.ProfileVersion)
		}
	})

	// An explicit "profile_version": 0 is not the same as an absent field:
	// zero is outside the supported range [1,1] and must be rejected the
	// same way 99 is, naming the declared version and the range — a *int
	// (rather than int with the zero value doubling as "absent") is what
	// lets Validate tell the two cases apart.
	t.Run("explicit zero is out of range, distinct from absent", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{"profile_version": 0, "node_types": []}`
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(doc), 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		_, err := ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error resolving an explicit profile_version: 0, got nil")
		}
		if !strings.Contains(err.Error(), "profile.json") {
			t.Fatalf("error should name the profile file, got: %v", err)
		}
		if !strings.Contains(err.Error(), "0") {
			t.Fatalf("error should name the declared version (0), got: %v", err)
		}
		if !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("error should say unsupported, not silently treat 0 as absent, got: %v", err)
		}
	})
}

// TestFR9_P9_EmbeddedDefaultProfileIsOrdinaryDocument covers
// test_schema_loading.md's P9: the embedded defaultProfile.json is decoded
// and validated through the exact same path a file-backed profile.json
// takes — not a privileged code path — and resolving it directly or via a
// copied spec/profile.json yields identical resolved profiles.
func TestFR9_P9_EmbeddedDefaultProfileIsOrdinaryDocument(t *testing.T) {
	data, err := defaultProfileFS.ReadFile("defaultProfile.json")
	if err != nil {
		t.Fatalf("read embedded defaultProfile.json: %v", err)
	}

	embedded, err := decodeProfile(data)
	if err != nil {
		t.Fatalf("decode embedded defaultProfile.json: %v", err)
	}
	if err := embedded.Validate(); err != nil {
		t.Fatalf("embedded defaultProfile.json should validate like any file-backed profile: %v", err)
	}
	embedded.finalize()

	if embedded.ProfileVersion == nil || *embedded.ProfileVersion != 1 {
		t.Fatalf("embedded defaultProfile.json should declare profile_version 1, got %v", embedded.ProfileVersion)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "profile.json"), data, 0o644); err != nil {
		t.Fatalf("write profile.json: %v", err)
	}
	fileBacked, err := ResolveProfile(dir)
	if err != nil {
		t.Fatalf("ResolveProfile over a copy of the embedded document: %v", err)
	}

	if !reflect.DeepEqual(embedded, fileBacked) {
		t.Fatal("resolving the embedded document directly and via a copied spec/profile.json should yield identical profiles")
	}
	if !reflect.DeepEqual(embedded, DefaultProfile()) {
		t.Fatal("DefaultProfile() should not be a privileged code path: it should equal the embedded document resolved the same way")
	}

	composedProject, err := ComposeProjectSchema(fileBacked.ProjectNodeTypes(), fileBacked.Edges)
	if err != nil {
		t.Fatalf("ComposeProjectSchema: %v", err)
	}
	shippedProject, err := projectSchemaBytes()
	if err != nil {
		t.Fatalf("projectSchemaBytes: %v", err)
	}
	if !jsonEqual(t, composedProject, shippedProject) {
		t.Fatal("composing from the file-backed copy of the embedded document should still reproduce the shipped project schema (P2 holds over both)")
	}
}

// TestFR9_P10_PlanRelevantValidatedAsOrderedListOfDeclaredTypes covers
// test_schema_loading.md's P10: plan_relevant is validated as an ordered
// list of declared types — a repeated entry or an undeclared entry fails
// early, exactly as P7's field-declaration defects do, while a reordering
// or an empty list are both legal declarations, and the resolved profile
// exposes the list in the declared order rather than just the membership.
func TestFR9_P10_PlanRelevantValidatedAsOrderedListOfDeclaredTypes(t *testing.T) {
	withPlanRelevant := func(t *testing.T, planRelevant any) []byte {
		t.Helper()
		data, err := defaultProfileFS.ReadFile("defaultProfile.json")
		if err != nil {
			t.Fatalf("read embedded defaultProfile.json: %v", err)
		}
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("unmarshal embedded defaultProfile.json: %v", err)
		}
		doc["plan_relevant"] = planRelevant
		out, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("marshal modified profile: %v", err)
		}
		return out
	}

	t.Run("a type listed twice fails early", func(t *testing.T) {
		dir := t.TempDir()
		doc := withPlanRelevant(t, []string{"component", "component", "data_flow", "test_section"})
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), doc, 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		_, err := ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "profile.json") {
			t.Fatalf("error should name the profile file, got: %v", err)
		}
		if !strings.Contains(err.Error(), `"component"`) || !strings.Contains(err.Error(), "already listed") {
			t.Fatalf("error should name the repeated entry, got: %v", err)
		}
	})

	t.Run("an undeclared type fails early", func(t *testing.T) {
		dir := t.TempDir()
		doc := withPlanRelevant(t, []string{"endpoint"})
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), doc, 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		_, err := ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "profile.json") {
			t.Fatalf("error should name the profile file, got: %v", err)
		}
		if !strings.Contains(err.Error(), `"endpoint"`) || !strings.Contains(err.Error(), "undeclared node type") {
			t.Fatalf("error should name the undeclared entry, got: %v", err)
		}
	})

	t.Run("a reordering resolves and preserves the declared order", func(t *testing.T) {
		dir := t.TempDir()
		doc := withPlanRelevant(t, []string{"test_section", "component", "data_flow"})
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), doc, 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		p, err := ResolveProfile(dir)
		if err != nil {
			t.Fatalf("a reordered plan_relevant should resolve cleanly: %v", err)
		}
		want := []string{"test_section", "component", "data_flow"}
		if !slices.Equal(p.PlanRelevant, want) {
			t.Fatalf("PlanRelevant = %v, want %v (declared order)", p.PlanRelevant, want)
		}
		if slices.Equal(p.PlanRelevant, DefaultProfile().PlanRelevant) {
			t.Fatal("the reordered profile's PlanRelevant should differ from the default's")
		}
	})

	t.Run("an empty list is a legal declaration that no type produces tasks", func(t *testing.T) {
		dir := t.TempDir()
		doc := withPlanRelevant(t, []string{})
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), doc, 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		p, err := ResolveProfile(dir)
		if err != nil {
			t.Fatalf("an empty plan_relevant should resolve cleanly: %v", err)
		}
		if len(p.PlanRelevant) != 0 {
			t.Fatalf("PlanRelevant = %v, want empty", p.PlanRelevant)
		}
	})
}

// TestFR3_FormatVersionDeclarations covers arch_schema_loader.md's format
// version declarations beyond profile_version (pinned separately by
// TestFR9_P8_ProfileVersionOutOfRangeFailsEarly): SchemaLoader carries the
// binary's supported spec_version and the journal-line version writers
// stamp, so a future spec_version check or journal writer has a constant to
// consume rather than a bare literal 1.
func TestFR3_FormatVersionDeclarations(t *testing.T) {
	if SupportedSpecVersion != 1 {
		t.Fatalf("SupportedSpecVersion = %d, want 1", SupportedSpecVersion)
	}
	if JournalLineVersion != 1 {
		t.Fatalf("JournalLineVersion = %d, want 1", JournalLineVersion)
	}
}
