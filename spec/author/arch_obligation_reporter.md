# ObligationReporter

The stage every writing command in this module passes through on its way to disk, and the reason none of them carries a rule of its own. It answers two questions about a change the caller has already applied to an in-memory copy of the spec: *may this be written* — [[6ce5fdd9e930|Refusals name the fix]] — and *what does writing it oblige* — [[11f21556b421|Report obligations before writing]].

## Refusal is the validator's predicate

The reporter runs the validator's own checkers over the after-state — [[651d5315eebf|SchemaChecker]] for conformance of the composed documents, [[00beeeda5ddd|IDValidator]] for id derivation, uniqueness and reference integrity, [[c6c770a59d68|DAGChecker]] for acyclicity of every cycle-checked field, the validator's `link` check — a check of the pipeline no component owns — for every typed link in every leaf, and the validator's name tokenization for declarability — and turns an entry they produce into a refusal when the before-state did not carry it. The entry is the validator's: the same `check` value and the same message a hand edit of the same shape would earn from `spex validate` on the copy. Nothing in this module restates a rule, and nothing decides a case the validator would decide otherwise. The two directions of that parity are both contracts: a change the validator would refuse is refused here before the file is touched, and a change the validator would accept — a project requirement nothing derives from yet, say — is written and its validator finding is reported as an obligation, not held back as a refusal.

The checkers run over an in-memory tree rather than a directory, which is the one thing the validator's interface had to admit for this module to exist: the checkers take a loaded spec, and `spex validate` is the caller that loads it from disk. A refusal therefore costs one validation pass over each state and no write. Where any checker refuses, nothing reaches disk — not the part of the change that was fine.

What is refused is what the change introduced: an entry present in the after-state and absent from the before-state. An entry the tree already carried before the command ran is not the change's fault and is not held against it — it travels in the report as an obligation, so a tree that fails validation can be edited towards passing it one command at a time, and a migration over a document that fails for reasons of its own still lands. This is the rule that lets the parity oracle read in both directions: the hand copy of an accepted change fails `spex validate` with exactly the entries the report listed as obligations, and with nothing the report called a refusal.

## Every refusal names its fix

A refusal is an error document on stdout, and each entry carries a `fix` field beside the validator's `check` and `message`: the command, flag or value that resolves it, in the shape [[c318df455dcc|`spex doctor`]] uses for its findings. The fix is computed from the profile and the tree, never from a table of messages:

| Refusal | Fix carried |
|---|---|
| a type the profile does not declare | the types it does declare, and `module` |
| a required field absent | the field's name and kind |
| a reference target that does not exist | the array that was searched, by file and key |
| a field the source type may not carry, or a target type it may not point at | the fields the profile declares on that type, or the target types that field permits |
| a name the tokenizer would not reproduce | the form it would reproduce |
| a duplicate id or name | the existing node's id and file |
| an inbound reference blocking a removal | the `spex edge remove` invocation that retargets it, and `--force` |
| a cycle | the cycle, as the validator's `dag` entry states it |

This is the property a short authoring skill depends on. A tool whose errors are terse makes the skill pre-empt them; a tool whose errors carry the fix lets the skill's loop be "run the command, apply the fix it names".

## Obligations are printed, not discovered

After the checkers pass, the reporter runs [[de3309dfbd3c|CompletenessChecker]] over the before-and-after pair the command produced — the tree as it was on disk and the tree as it is about to be written, hashed the way `spex diff` hashes them — and prints the entries it returns under an `obligations` key in the write report. They are the completeness checker's entries, unchanged: a requirement added or changed obliges every implementing component's leaf; a `module.json` change moves the module's `meta` leaf and obliges every component in the module unless a requirement in it also changed; a component whose edges changed owes its own leaf. `spex diff --json` run afterwards against a snapshot taken before the command reports the same entries, entry for entry, in its `errors` array — the two are the same computation over the same pair, and the reporter exists so that the agent sees the cost when it is incurred rather than at the end of the session.

Obligations never block a write. They are what the write leaves open, and the validator's own findings on an accepted change — the underived project requirement above — travel in the same array for the same reason.

## What it does not do

It does not read `.spex/`: the before-state is the tree on disk, not the baseline snapshot, so the obligations of one command are relative to the previous command's result and a session of commands accumulates the same set `spex diff` will report at the end. It does not write: the caller writes, after the reporter has answered, or does not. And it carries no profile knowledge of its own beyond what the checkers and the completeness rules already read from the resolved profile handed to it.
