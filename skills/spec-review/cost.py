#!/usr/bin/env python3
"""Cost figures of a subagent run, from its transcript.

usage: cost.py <transcript.jsonl> [<transcript.jsonl> ...]

Per transcript: tool calls, turns, final context (input + cache read + cache
creation of the last turn), output tokens, turns times context (the volume the
bill scales with), and the cache split — how many input tokens were created
versus read from cache across the run, and the same split over the reading
phase alone (the turns up to the last one whose tool result is a staged file
read). A second reviewer on an identical read plan shows its reading phase as
cache reads; a first reviewer shows it as cache creation.
"""
import json, sys

def figures(path):
    turns, calls, chars, reading_end = [], 0, 0, 0
    for line in open(path, errors="replace"):
        try: ev = json.loads(line)
        except Exception: continue
        if not isinstance(ev, dict): continue
        m = ev.get("message") or ev
        if isinstance(m, dict) and isinstance(m.get("usage"), dict):
            u = m["usage"]
            turns.append((u.get("input_tokens", 0), u.get("cache_read_input_tokens", 0), u.get("cache_creation_input_tokens", 0), u.get("output_tokens", 0)))
        c = m.get("content") if isinstance(m, dict) else None
        if not isinstance(c, list): continue
        for b in c:
            if not isinstance(b, dict): continue
            if b.get("type") == "tool_use":
                calls += 1
                cmd = (b.get("input", {}) or {}).get("command", "") or ""
                if "cat " in cmd and ("READ-PLAN" in cmd or "$STAGE" in cmd or "/stage" in cmd): reading_end = len(turns)
            elif b.get("type") == "tool_result":
                x = b.get("content"); x = " ".join(y.get("text", "") for y in x if isinstance(y, dict)) if isinstance(x, list) else (x or "")
                chars += len(x)
    if not turns: return None
    final = sum(turns[-1][:3]); ctx_sum = sum(sum(t[:3]) for t in turns)
    created, read = sum(t[2] for t in turns), sum(t[1] for t in turns)
    r_created, r_read = sum(t[2] for t in turns[:reading_end]), sum(t[1] for t in turns[:reading_end])
    return dict(calls=calls, turns=len(turns), chars=chars, final=final, output=sum(t[3] for t in turns), volume=ctx_sum,
                created=created, read=read, reading_turns=reading_end, reading_created=r_created, reading_read=r_read)

def main(paths):
    for p in paths:
        f = figures(p)
        if not f: print(f"{p}: no usage records"); continue
        pct = lambda a, b: f"{100 * a / (a + b):.0f}%" if a + b else "n/a"
        print(f"{p.rsplit('/', 1)[-1]}: calls {f['calls']}, turns {f['turns']}, tool-result chars {f['chars']}, final context {f['final']}, output {f['output']}, turns x context {f['volume'] / 1e6:.1f}M")
        print(f"  cache: created {f['created']} / read {f['read']} (created {pct(f['created'], f['read'])}); reading phase ({f['reading_turns']} turns): created {f['reading_created']} / read {f['reading_read']} (created {pct(f['reading_created'], f['reading_read'])})")
    return 0

if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
