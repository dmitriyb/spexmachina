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
	for _, key := range []string{"name", "description", "version", "requirements", "modules", "sections"} {
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
		{"bead-map", BeadMapSchema},
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

// --- BeadMapSchema tests (BM1-BM2) ---

func TestFR7_BM1_BeadMapSchemaLoads(t *testing.T) {
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	if len(data) == 0 {
		t.Fatal("BeadMapSchema() returned empty bytes")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestFR7_BM2_BeadMapAcceptsBothIdentitiesNodeCarries(t *testing.T) {
	schData, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	sch := compileSchema(t, schData)

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
		t.Fatal("empty node should fail validation")
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
	for _, nt := range p.NodeTypes {
		names[nt.Name] = true
		if nt.CompletenessTrigger {
			completenessTrigger[nt.Name] = true
		}
		if nt.NameDeclarable {
			nameDeclarable[nt.Name] = true
		}
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

	if len(p.Edges) != 7 {
		t.Fatalf("edges = %d, want 7", len(p.Edges))
	}
	wantEdgeKinds := map[string]bool{
		"preq_id": true, "implements": true, "uses": true, "provided_by": true,
		"describes": true, "depends_on": true, "requires_module": true,
	}
	for _, e := range p.Edges {
		if !wantEdgeKinds[e.Kind] {
			t.Fatalf("unexpected edge kind %q", e.Kind)
		}
		if e.Cyclic {
			t.Fatalf("edge %q should not carry the cyclic exemption under the default profile", e.Kind)
		}
	}

	if len(p.CoverageChains) != 3 {
		t.Fatalf("coverage chains = %d, want 3", len(p.CoverageChains))
	}

	wantPlanRelevant := map[string]bool{"component": true, "data_flow": true, "test_section": true}
	if len(p.PlanRelevant) != len(wantPlanRelevant) {
		t.Fatalf("plan-relevant set = %v, want %v", p.PlanRelevant, wantPlanRelevant)
	}
	for _, n := range p.PlanRelevant {
		if !wantPlanRelevant[n] {
			t.Fatalf("plan-relevant set has unexpected member %q", n)
		}
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
		"project:requirement": {"depends_on", "description", "id", "priority", "title", "type"},
		"module:requirement":  {"depends_on", "description", "id", "preq_id", "title", "type"},
		"module:api":          {"description", "group", "id", "name", "provided_by"},
	}
	if !reflect.DeepEqual(p.HashedFields, wantHashedFields) {
		t.Fatalf("hashed field allowlists = %v, want %v", p.HashedFields, wantHashedFields)
	}

	wantAbsorbable := map[string]AbsorbDirections{
		"requirement":  {Added: true, Removed: true},
		"api":          {Added: true, Removed: true},
		"component":    {Added: false, Removed: true},
		"data_flow":    {Added: false, Removed: false},
		"test_section": {Added: false, Removed: false},
	}
	if !reflect.DeepEqual(p.Absorbable, wantAbsorbable) {
		t.Fatalf("absorbable directions = %v, want %v", p.Absorbable, wantAbsorbable)
	}
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
			],
			"edges": [],
			"coverage_chains": [],
			"plan_relevant": [],
			"impact_levels": {},
			"hashed_fields": {},
			"absorbable": {}
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

	t.Run("hashed_fields key is not scope:name", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{
			"node_types": [
				{"name": "widget", "plural_key": "widgets", "scope": "module"}
			],
			"edges": [],
			"coverage_chains": [],
			"plan_relevant": [],
			"impact_levels": {},
			"hashed_fields": {"widget": ["id"]},
			"absorbable": {}
		}`
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(doc), 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		_, err := ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error resolving a profile with a malformed hashed_fields key, got nil")
		}
		if !strings.Contains(err.Error(), "profile.json") {
			t.Fatalf("error should name the profile file, got: %v", err)
		}
		if !strings.Contains(err.Error(), "hashed_fields") {
			t.Fatalf("error should name the defect (hashed_fields), got: %v", err)
		}
	})

	t.Run("hashed_fields scope:name typo is not a declared type", func(t *testing.T) {
		dir := t.TempDir()
		doc := `{
			"node_types": [
				{"name": "widget", "plural_key": "widgets", "scope": "module"}
			],
			"edges": [],
			"coverage_chains": [],
			"plan_relevant": [],
			"impact_levels": {},
			"hashed_fields": {"module:gadget": ["id"]},
			"absorbable": {}
		}`
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), []byte(doc), 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}
		_, err := ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error resolving a profile with an undeclared hashed_fields node type, got nil")
		}
		if !strings.Contains(err.Error(), "gadget") {
			t.Fatalf("error should name the undeclared type (gadget), got: %v", err)
		}
	})
}

// TestFR9_P4_DeclaredCustomTypeReachesComposedSchema declares a custom
// "endpoint" type in spec/profile.json and checks that it reaches the
// composed module schema with the same generic envelope constraints
// built-in types get, while additionalProperties:false still rejects any
// array the profile does not declare.
func TestFR9_P4_DeclaredCustomTypeReachesComposedSchema(t *testing.T) {
	profile := DefaultProfile()
	profile.NodeTypes = append(profile.NodeTypes, NodeType{
		Name:            "endpoint",
		PluralKey:       "endpoints",
		Scope:           "module",
		RequiresContent: true,
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
	sch := compileSchemaFromBytes(t, composed)

	if err := validateModule(t, sch, `{
		"name": "m",
		"endpoints": [
			{"id": "aabbccddeeff", "name": "GET /things", "content": "endpoint_things.md"}
		]
	}`); err != nil {
		t.Fatalf("profile-declared endpoints array should pass: %v", err)
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

// TestFR9_P6_NewEdgeKindSourcedAtBuiltinTypeRejected covers arch_profile_loader.md's
// rule that a profile-declared edge kind the frame does not already carry is
// refused at validation when its source is a built-in type — the built-in
// $defs are frame-fixed and gain no new reference fields in composition, so
// such an edge could never be carried by any document.
func TestFR9_P6_NewEdgeKindSourcedAtBuiltinTypeRejected(t *testing.T) {
	t.Run("new edge kind sourced at a built-in type is refused", func(t *testing.T) {
		profile := DefaultProfile()
		profile.NodeTypes = append(profile.NodeTypes, NodeType{
			Name:            "endpoint",
			PluralKey:       "endpoints",
			Scope:           "module",
			RequiresContent: true,
		})
		profile.Edges = append(profile.Edges, Edge{
			Kind: "audits",
			From: []string{"component"},
			To:   []string{"endpoint"},
		})

		dir := t.TempDir()
		data, err := json.Marshal(profile)
		if err != nil {
			t.Fatalf("marshal profile: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "profile.json"), data, 0o644); err != nil {
			t.Fatalf("write profile.json: %v", err)
		}

		_, err = ResolveProfile(dir)
		if err == nil {
			t.Fatal("expected an error resolving a profile with a new edge kind sourced at a built-in type, got nil")
		}
		if !strings.Contains(err.Error(), "profile.json") {
			t.Fatalf("error should name the profile file, got: %v", err)
		}
		if !strings.Contains(err.Error(), "audits") {
			t.Fatalf("error should name the declaration (audits), got: %v", err)
		}
	})

	t.Run("an edge kind the frame already carries remains declarable at a built-in type", func(t *testing.T) {
		// "uses" is already carried by the frame at "component" (and
		// "data_flow") in DefaultProfile — redeclaring it, even with an
		// extra To target, must not trip the new-edge-kind rule.
		profile := DefaultProfile()
		for i, e := range profile.Edges {
			if e.Kind == "uses" {
				profile.Edges[i].To = append(append([]string{}, e.To...), "requirement")
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

		if _, err := ResolveProfile(dir); err != nil {
			t.Fatalf("expected the already-carried edge kind %q at a built-in type to resolve cleanly, got: %v", "uses", err)
		}
	})
}
