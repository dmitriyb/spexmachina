package author

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/dmitriyb/spexmachina/schema"
)

// ShowProfile is ProfileInspector: it resolves the profile for specDir and
// writes it to w as one JSON document — compact, or indented two spaces
// when pretty is true — per spec/author/arch_profile_inspector.md "What it
// prints". Resolution is schema's own (schema.ResolveProfile): the
// built-in default, or spec/profile.json when the project commits one,
// validated the same way every other command validates it, so a malformed
// or out-of-range profile fails here with the schema module's one message
// naming the file, its version and the supported range — never a
// re-implementation of that check. ShowProfile writes nothing under
// spec/, needs no initialised project and reads nothing under .spex/; it
// returns the resolved profile alongside the write for a caller that wants
// both.
func ShowProfile(specDir string, w io.Writer, pretty bool) (*schema.Profile, error) {
	profile, err := schema.ResolveProfile(specDir)
	if err != nil {
		return nil, err
	}

	enc := json.NewEncoder(w)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(profile); err != nil {
		return nil, fmt.Errorf("author: encode profile: %w", err)
	}
	return profile, nil
}
