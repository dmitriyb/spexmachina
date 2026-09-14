# LeafScaffolder

The component behind `spex leaf scaffold`, and the one `spex node add` calls for a content-bearing node. It writes the skeleton of a content leaf — what a leaf of that type carries before any prose is written — from the profile's declarations and the node's declared edges, so that the headings the review lenses expect and the links the leaf owes are on disk before the agent starts writing. [[1631cb19fac3|Scaffold content leaves]] is its contract.

## What a skeleton is

For a node whose type is content-bearing, at the node's declared `content` path:

1. the title heading, `# <name>`;
2. one `##` heading per entry in the type's `leaf_sections` declaration, in declared order — under the default profile the headings the arch, flow and test leaves of this repository already share; a type declaring none gets no `##` heading at all;
3. one placeholder line per declared edge the leaf owes a link for — for a component, every entry of its `implements` and `uses` and every api whose `provided_by` names it; for a data flow, its `uses` — each carrying the typed link to that target the obligation scanner will look for, so that a leaf scaffolded and then written satisfies the link obligation by construction and the placeholder is replaced by the sentence that discusses the target.

The headings come from the profile, never from the scaffolder: it names no section and no type. A project whose profile declares `endpoint` with sections `Contract` and `Errors` gets those two, and `spex profile show` is where an agent reads them before writing.

## Refusals and idempotence

A leaf that exists and is non-empty is never overwritten: the refusal names the file and says to empty or move it, because prose is the one thing in the tree the tool must not destroy. A leaf that is absent or zero bytes long is written. A leaf holding exactly the skeleton the scaffolder would write is reported as already scaffolded and left byte-identical — the second run of an idempotent command changes nothing and says so.

A node of a type the profile marks as not content-bearing — an api under the default profile — has no leaf to scaffold; the refusal lists the content-bearing types. A node that does not exist is refused with the array that was searched.

Scaffolding writes a leaf and changes no JSON, so it passes through [[b9e7b96f6aa7|ObligationReporter]] for the write report's shape and the parity check on the tree, and it reports no obligations of its own: a written leaf is what discharges an obligation, never what incurs one.

## The line it does not cross

The skeleton is the structural half of a leaf. The prose is the creative half and is written by hand, in the file the scaffolder left — the only thing in a scaffolded spec an agent writes directly. Nothing here generates a sentence.
