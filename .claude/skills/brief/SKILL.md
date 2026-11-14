---
name: brief
description: Turn a rough tana feature idea into a behaviour brief to paste into /ship-feature.
argument-hint: "rough feature idea"
disable-model-invocation: true
---

Turn the rough idea below into a **brief** for `/ship-feature`. The `planner` agent settles everything the brief leaves open, so this skill ends at the brief, with the workflow unstarted.

<rough_idea>
$ARGUMENTS
</rough_idea>

## Steps

1. **Read the code** the idea touches, hunting **traps**: today's behaviour that would make the obvious version hurt the user. Read `CONTEXT.md` and the ADRs the idea touches first. Done when you can say what happens today on the feature's main path and its failure path, and which ADR, if any, the idea contradicts.
2. **Walk the feature in use**: the main path, each failure (an unavailable library root, the collector unreachable or banned, an empty result), and each handover, where a scan, download or sync is still running as the state changes. Map the **user decisions** as a **design tree**: choices that change what the user gets, where reasonable users would differ, each branching into the decisions that hang off it. Leave every other choice to the planner.
3. **Ask in rounds.** The **frontier** is every open user decision whose prerequisites are settled. Ask the frontier with `AskUserQuestion`, up to 4 questions per call and the recommended option first; split a larger frontier over several calls, the most consequential decisions first. A question that depends on another still open in this round waits for a later round. Each question names the trap or situation behind it, because the dialog may hide your earlier text.
   - **Facts are yours**: look up anything the code, docs or tools can answer, and ask the user only for decisions.
   - After each round, fold the answers into the tree and walk the affected paths again: answers push the frontier outward, open new decisions, and settle others. Recompute the frontier and ask the next round.
   - Delegate a decision to the planner once a reasonable default covers it and the user would not care which way it goes.

   Done when the frontier is empty: every branch visited, each user decision answered or delegated, nothing silently assumed.
4. **Write the brief** in one code block. After it, add a line for anything in the brief you didn't discuss, and nothing else.

## The brief

- Open with a title and one sentence on what the user can do.
- Describe what the user sees and does in the web UI: views, actions, messages, failures, grouped under short headings. Name domain concepts with `CONTEXT.md` terms.
- Include every user decision as its outcome, with exclusions the user chose ("no bulk delete"). Give the intent and leave exact wording to the planner ("the empty library says how to add a root").
- State a guard's intent ("a refresh never removes a source from an unavailable library"), and leave its mechanism and timings to the planner.
- Give each trap one line naming the code. Name a contradicted ADR with the user's decision on it.
- Everything else is the planner's: packages, routes, schema, tests, manual checks, and the wider out-of-scope list.
