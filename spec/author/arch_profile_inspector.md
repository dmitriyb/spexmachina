# ProfileInspector

The component behind `spex profile show`: the one read surface of this module, and the printer every generic authoring skill is written against. [[7f193910f7ef|Show the resolved profile]] is its contract.

## Why a printer

Until this component existed the resolved profile had no dedicated command; it was observable only through the node table `spex render` emits. A skill that must never name a node type has nothing to read the types from, so the printer is the precondition for every command in this module being usable without memorised knowledge: an agent runs `spex profile show`, reads the declared types, their plural keys, their fields with kinds and reference targets, the coverage chains, the plan-relevant list, and per content-bearing type the content prefix and the leaf sections, and then declares nodes it has never seen the names of.

## What it prints

The resolved profile as one JSON document on stdout — compact when piped, pretty-printed on a terminal, and nothing around it. Resolution is the schema module's: the built-in default, or `spec/profile.json` when the project commits one, after the same validation every other command applies, so a malformed or out-of-range profile fails here with the same single message it fails with everywhere — the file, its version and the supported range. A version 1 profile is printed with the version 2 conventions filled in, `content_prefix` and `leaf_sections` carrying the defaults, because that is what resolution yields for it; the printed document is the resolved one, not the file's bytes.

The document round-trips: written to `spec/profile.json` and resolved again, it prints equal as a JSON value. That is the property the golden test holds it to, and it is what makes the printed document a safe starting point for a project authoring its own profile.

## What it does not do

It writes nothing, reports no obligations, needs no initialised project and reads nothing under `.spex/`. It is the only command in this module that never touches [[b9e7b96f6aa7|ObligationReporter]], which is why it declares no use of it.
