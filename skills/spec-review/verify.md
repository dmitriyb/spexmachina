# Verification pass — instructions for the verifier subagent

You verify findings; you do not review. Input: a stage directory `$STAGE` holding staged spec
files and `$STAGE/FINDINGS.tsv` (columns: `id`, `left_id`, `left_file`, `left_quote`, `right_id`,
`right_file`, `right_quote`, `contradiction`, `fix`). You read nothing under `spec/` and nothing
outside the stage; you write exactly one file, `$STAGE/VERDICTS.tsv`.

The passages are already cut: `$STAGE/PASSAGES.tsv` (columns `id`, `side`, `file`, `quote`,
`passage`) holds, per quote, the block it sits in with the block before and after and the nearest
heading, separated by ` || `. Read that file once; it is the whole input. Open a staged file only
when a passage says `NOT FOUND` or `FILE NOT STAGED`, or when the cut blocks are not enough to
decide the question below, and say so in the reason.

For each finding, in order:

1. Read its two passages (or the one, for a one-sided finding).
2. Answer one question: **does the contradiction exist as the `contradiction` sentence states
   it?** Not whether the fix is good, not whether the leaf could be better, not whether you would
   have phrased the finding differently.
3. Write one row: `id`, verdict, reason.
   - `holds` — the two passages say incompatible things, or the one passage makes the claim the
     finding says it makes (for a one-sided finding), exactly as stated.
   - `does-not-hold` — read in context, the passages are compatible (one narrows the other and
     says so, they speak of different cases, the "contradiction" is a different level of detail).
   - `mistake` — a quote is not where the finding says, the finding misreads a passage, or the
     contradiction as stated is about something neither passage says.
   The reason is one sentence and quotes the words that decide it.

Rules: a verdict without a quote in its reason is not a verdict. Do not soften a `mistake` into
`does-not-hold`. Do not add findings. Do not propose fixes. When done, reply with the counts of
each verdict and nothing else; the file is the deliverable.
