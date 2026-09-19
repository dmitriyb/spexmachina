#!/usr/bin/env python3
"""Cut the passages a verifier needs from the stage: for every row of
FINDINGS.tsv, the paragraph holding each quote plus the paragraph before and
after it, and the nearest heading above. Writes PASSAGES.tsv beside it.

usage: passages.py <stage-dir>

A quote that cannot be located is written with the passage `NOT FOUND`, which
the verifier must report as a mistake. Exit 0 always; the audit already checks
quotes.
"""
import os, re, sys

def norm(t): return re.sub(r"\s+", " ", t).strip()
def unq(f):
    """A field a CSV writer wrapped in double quotes, inner quotes doubled: undo it."""
    f = f.strip("\r")
    return f[1:-1].replace('""', '"') if len(f) >= 2 and f[0] == '"' and f[-1] == '"' else f

def json_strings(obj, out):
    if isinstance(obj, dict):
        for v in obj.values(): json_strings(v, out)
    elif isinstance(obj, list):
        for v in obj: json_strings(v, out)
    elif isinstance(obj, str): out.append(obj)
    return out

def passage(path, quote):
    if not os.path.exists(path): return "FILE NOT STAGED"
    text = open(path, encoding="utf-8", errors="replace").read()
    if path.endswith(".json"):  # the passage of a JSON quote is the field's decoded string
        try:
            import json
            for s_ in json_strings(json.loads(text), []):
                if norm(quote) in norm(s_):
                    ns, q = norm(s_), norm(quote); pos = ns.find(q); lo = max(0, pos - 500); hi = min(len(ns), pos + len(q) + 500)
                    return ("… " if lo else "") + ns[lo:hi] + (" …" if hi < len(ns) else "")
        except Exception: pass
    # a block is a paragraph, a list item, a heading or a table row: split on blank lines and
    # on lines that open a bullet, a numbered item, a heading or a table row
    paras = [p for p in re.split(r"\n\s*\n|\n(?=\s*(?:[-*+] |\d+\. |#{1,6} |\|))", text) if p.strip()]
    q = norm(quote)
    for i, p in enumerate(paras):
        if q and q in norm(p):
            heading = next((h for h in reversed(paras[:i]) if h.lstrip().startswith("#")), "")
            before = paras[i - 1] if i > 0 else ""
            after = paras[i + 1] if i + 1 < len(paras) else ""
            # bound the passage: the quote's block windowed to 1200 chars around the quote, neighbours to 300
            np_ = norm(p); pos = np_.find(q); lo = max(0, pos - 500); hi = min(len(np_), pos + len(q) + 500)
            block = ("… " if lo else "") + np_[lo:hi] + (" …" if hi < len(np_) else "")
            cut = lambda t, n: (norm(t)[:n] + " …") if len(norm(t)) > n else norm(t)
            return " || ".join(x for x in (cut(heading, 120), cut(before, 300), block, cut(after, 300)) if x)
    return "NOT FOUND"

def main(stage):
    rows = [[unq(x) for x in l.rstrip("\n").split("\t")] for l in open(os.path.join(stage, "FINDINGS.tsv"), encoding="utf-8")]
    out = open(os.path.join(stage, "PASSAGES.tsv"), "w", encoding="utf-8")
    out.write("id\tside\tfile\tquote\tpassage\n")
    n = 0
    for c in rows[1:]:
        if len(c) < 9: continue
        for side, f, q in (("left", c[2], c[3]), ("right", c[5], c[6])):
            if not q.strip(): continue
            out.write("\t".join([c[0], side, f, q, passage(os.path.join(stage, f), q)]) + "\n"); n += 1
    print(f"passages: {n} written to PASSAGES.tsv for {len(rows) - 1} findings")

if __name__ == "__main__":
    sys.exit(main(sys.argv[1].rstrip("/")))
