package delivery

import _ "embed"

// InstallScript is the embedded copy of the install/self-update script
// spec/delivery/arch_self_update.md describes. It is kept byte-identical
// to the canonical copy at the repository root (install.sh) by
// TestInstallScript_EmbeddedMatchesReleased — a build-failing test, not a
// runtime check, so drift between the two cannot survive a build. Nothing
// is fetched to obtain this script at upgrade time: it is already inside
// the trusted, signed binary that embeds it.
//
//go:embed install.sh
var InstallScript string
