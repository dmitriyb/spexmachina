# InitCommand

The entry point behind the [[063aeb01cdfe|`spex init`]] api: the moment a spex project is born. It creates the `.spex/` state directory, per [[ac57844f66a6|Initialize a project]], and nothing else acquires that power — no other command initialises, implicitly or otherwise.

## Behaviour

`spex init` writes exactly two files into a fresh `.spex/`:

- **The snapshot, seeded with the canonical empty tree** — never with a snapshot of the spec that already exists. Seeding from the current spec would make the first diff clean, and no work would ever be born from the initial spec. The empty tree is produced here and only here; the snapshot loader no longer invents it as a fallback for a missing file.
- **An empty journal, with deliberately no init event.** The empty journal *is* the fact "no cycle has completed", and the refresh bootstrap guard in ingest reads exactly that predicate. An event written at birth would make it permanently false. Provenance recorded at birth is a real thing to want — but not at the cost of the fact the rest of the system reads.

After `init`, the shortest useful sequence needs no tracker and no adapter: `spex validate`, `spex diff` and `spex render` all answer real questions read-only, and the first `spex diff` reports the entire spec as added — the intended bootstrap, now distinguishable from an accident because an uninitialised directory is refused instead.

## Refusal

`spex init` refuses a directory that already has `.spex/`, whatever its condition, and leaves it untouched. It is the one command that can destroy a journal — the single artifact that cannot be reconstructed from anything else — so it never overwrites; a broken state directory is `spex doctor`'s to diagnose and a human's to resolve. It consults [[a9aa93774cc2|ProjectResolver]] to classify the directory's current state rather than re-deriving the rules; the resolver's "never initialised" answer is init's precondition.

## The committed directory

`.spex/` is committed to git, not ignored. Init's job includes making that survivable: the directory it creates is exactly what the journal's durability story assumes, and a `.gitignore` habit that swallows dotfiles is the failure mode to check for when adopting spex in a new repository.
