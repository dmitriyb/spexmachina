package delivery

import "testing"

// TestInstallScript_EmbeddedMatchesReleased pins the embedded copy of the
// install script to the canonical copy at the repository root, satisfying
// [[5b33ea62c4e3|Embedded installer byte-identical to the released one]]:
// a one-byte divergence between the two fails the build.
func TestInstallScript_EmbeddedMatchesReleased(t *testing.T) {
	released := readRepoFile(t, "install.sh")
	if InstallScript != released {
		t.Fatalf("delivery/install.sh (embedded) does not match install.sh (released copy at repository root); they must be kept byte-identical")
	}
}
