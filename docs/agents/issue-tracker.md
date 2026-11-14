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

## Feature tickets

`/ship-feature` (`.claude/workflows/ship-feature.js`) builds a feature
from these files on the branch `feature/<feature-slug>`, so
`/code-review` finds the spec by branch name. Its final review appends
work beyond the spec to `follow-ups.md`.

Ticket format, numbered in dependency order, blockers first:

```markdown
# NN: <Ticket title>

What to build: the end-to-end behaviour this ticket makes work, from the user's side.

Blocked by: NN, NN (or None)

Status: ready-for-agent

- [ ] Acceptance criterion: an observable a test can check
```

- Statuses: `ready-for-agent`, `claimed`, `resolved`, or
  `ready-for-human`. The agent that changes a status commits the change
  with its work.
- Resolve: append `## Resolution` naming what changed.
- Handoff: an implementer that stops early appends `## Handoff` (what is
  committed, what remains, the next step). The next implementer starts
  there and deletes the section on resolve.
- Question: a ticket set to `ready-for-human` states its open design
  question under `## Comments`.
