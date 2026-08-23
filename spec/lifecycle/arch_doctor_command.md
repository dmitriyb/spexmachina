# DoctorCommand

The entry point behind the [[c318df455dcc|`spex doctor`]] api, and the answer to the first question anyone arriving at spex asks: *is my project set up correctly?* Per [[1ddbd6e36681|Diagnose project health]], it reports and never repairs.

## Behaviour

`spex doctor` examines the project state and reports, per artifact:

- what is **present** and readable;
- what is **missing**;
- what is **unreadable** — exists but cannot be parsed;
- and, for every finding, **the command that would fix it**. A project that was never initialised points at `spex init`; damage inside an existing state directory does not, because re-initialising is how a journal dies.

It builds on [[a9aa93774cc2|ProjectResolver]] for location resolution and the two-absence distinction, then goes deeper than the pre-flight does: the resolver stops at the first refusal, doctor enumerates every finding in one pass.

## What doctor never does

Doctor never mints, moves or repairs a baseline — not by default, and not behind a repair flag, because the flag is the failure mode: the moment doctor can rewrite a snapshot, "the baseline moves only deliberately" acquires an automated exception, and that exception would be exercised in exactly the situation — a broken state, a confused operator — where nobody is thinking clearly. Doctor diagnoses; a human decides. After any doctor run, the project directory is byte-identical to before.

## Exit behaviour

A healthy project exits 0. An unhealthy one exits non-zero with the findings on output, so CI can gate on project health without parsing prose.
