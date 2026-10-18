# Issue tracker: Local Markdown

Issues and specs live under `.scratch/<feature-slug>/`.

- Spec: `spec.md`.
- Tickets: `issues/<NN>-<slug>.md`, numbered from `01`, one per file.
- Triage state: `Status:` near the top; use `triage-labels.md`.
- Conversation: append under `## Comments`.

When publishing, create the appropriate spec or ticket file.
When fetching, read the referenced file; resolve numbers within the feature.

## Wayfinding

- Map: `.scratch/<effort>/map.md`, containing Notes, Decisions-so-far, and Fog.
- Children: `issues/<NN>-<slug>.md`, with the question in the body.
- Type: `research`, `prototype`, `grilling`, or `task`.
- Dependencies: `Blocked by: NN, NN`; unblocked when all are resolved.
- Frontier: open, unblocked, unclaimed tickets, lowest number first.
- Claim: save `Status: claimed` before working.
- Resolve: append `## Answer`, set `Status: resolved`, then add a gist
  and ticket link to the map's Decisions-so-far.
