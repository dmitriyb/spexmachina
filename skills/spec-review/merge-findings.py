#!/usr/bin/env python3
"""Union the FINDINGS.tsv files of several independent reviews of the same change.

usage: merge-findings.py <out-stage> <stage> [<stage> ...]

Rows are the same finding when they share a quote of substance (over 30
characters, whitespace-normalised, one containing the other), or when they share a node and their
contradiction sentences share at least a third of their content words — two
reviewers quote different sentences and name different second nodes for one
defect more often than not. Over-merging two distinct defects on one node is the
risk; the verifier reads the merged row's quotes, so a wrong merge surfaces there. The union is written to <out-stage>/FINDINGS.tsv
with ids re-numbered M1.., a `sources` column appended naming the stages that
produced each row, and the staged files of the first stage copied into
<out-stage> so passages.py and the verifier work on it unchanged.
"""
import os, re, shutil, sys

def norm(t): return re.sub(r"\s+", " ", t).strip().lower()
def unq(f):
    """A field a CSV writer wrapped in double quotes, inner quotes doubled: undo it."""
    f = f.strip("\r")
    return f[1:-1].replace('""', '"') if len(f) >= 2 and f[0] == '"' and f[-1] == '"' else f
STOP = set("the a an and or of to in is are it its that this with for on as by not no be at from which one two".split())

def main(argv):
    out, stages = argv[0].rstrip("/"), [s.rstrip("/") for s in argv[1:]]
    if not stages: print(__doc__); return 2
    if not os.path.exists(out):
        shutil.copytree(stages[0], out, ignore=shutil.ignore_patterns("FINDINGS.tsv", "VERDICTS.tsv", "PASSAGES.tsv", "CELLS.tsv", "PAIRS.tsv"))
    merged, keys = [], {}
    for st in stages:
        for l in open(os.path.join(st, "FINDINGS.tsv"), encoding="utf-8"):
            if l.startswith("id\t"): continue
            c = [unq(x) for x in l.rstrip("\n").split("\t")]
            if len(c) < 9: continue
            k = tuple(sorted((norm(c[3]), norm(c[6]))))
            quotes = {q for q in (norm(c[3]), norm(c[6])) if len(q) > 30}
            nodes_ = {x for x in (c[1], c[4]) if x}
            words = set(re.findall(r"[a-z0-9_]+", norm(c[7]))) - STOP
            hit = keys.get(k)
            if hit is None:
                for j, m in enumerate(merged):
                    mq = {q for q in (norm(m[3]), norm(m[6])) if len(q) > 30}
                    mn = {x for x in (m[1], m[4]) if x}
                    mw = set(re.findall(r"[a-z0-9_]+", norm(m[7]))) - STOP
                    # one shared quote of substance, or a shared node and contradictions sharing a third of their content words
                    same_quote = any(a in b or b in a for a in quotes for b in mq)  # one reviewer quotes a longer span than the other
                    if same_quote or (nodes_ & mn and words and mw and len(words & mw) / len(words | mw) >= 0.3): hit = j; break
            if hit is not None:
                if os.path.basename(st) not in merged[hit][9].split(","): merged[hit][9] += "," + os.path.basename(st)
                continue
            keys[k] = len(merged); merged.append(c[:9] + [os.path.basename(st)])
    with open(os.path.join(out, "FINDINGS.tsv"), "w", encoding="utf-8") as f:
        f.write("id\tleft_id\tleft_file\tleft_quote\tright_id\tright_file\tright_quote\tcontradiction\tfix\tsources\n")
        for i, c in enumerate(merged, start=1):
            c[0] = f"M{i}"; f.write("\t".join(c) + "\n")
    shared = sum(1 for c in merged if "," in c[9])
    print(f"merged: {len(merged)} findings from {len(stages)} reviews; found by more than one: {shared}")
    return 0

if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
