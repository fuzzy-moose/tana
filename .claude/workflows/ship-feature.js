export const meta = {
  name: 'ship-feature',
  description: 'Plan (fable, high), then implement, review and fix every ticket (medium) of a tana feature',
  whenToUse:
    'Pass a feature brief to plan and build it hands-off, or the slug of an existing .scratch/<feature>/ plan to build or resume it.',
  phases: [
    { title: 'Setup', detail: 'clean tree, find an existing plan' },
    { title: 'Plan', detail: 'planner writes spec + tickets, critic reviews, planner revises', model: 'fable' },
    { title: 'Tickets', detail: 'per ticket: implement → review → fix' },
    { title: 'Final review', detail: '/code-review of the branch, triage every open finding, apply the relevant ones' },
  ],
}

// The script is the orchestrator, and it holds no model context. Every agent() call starts
// fresh and returns a small structured result; durable state lives in the plan files and git,
// so re-running with the feature slug picks up where the last run stopped.

const input = (typeof args === 'string' ? args : (args?.brief ?? args?.feature ?? '')).trim()
if (!input) throw new Error('Pass a feature brief, or the slug of an existing plan')

// Model and effort per role. Agent frontmatter sets the same values for manual use; passing
// them here makes the pipeline independent of the session's own settings.
const PLAN = { model: 'fable', effort: 'high' }
const CRITIC = { model: 'opus', effort: 'high' }
const WORK = { model: 'opus', effort: 'medium' }
const CHORE = { model: 'haiku', effort: 'medium' } // setup, branch and the ticket board: git state and status lines
const JUDGE = { model: 'opus', effort: 'high' } // the one triage call decides what every leftover finding is worth
const MAX_HANDOFFS = 3 // fresh implementers per ticket after the first one hands off
const MAX_FIX_ROUNDS = 2

const str = { type: 'string' }
const strs = { type: 'array', items: str }
const SETUP = {
  type: 'object',
  properties: {
    ok: { type: 'boolean' },
    problem: str,
    existingPlan: { type: 'string', description: 'slug of the matching .scratch/<slug>/, or empty' },
  },
  required: ['ok', 'existingPlan'],
}
const PLANNED = {
  type: 'object',
  properties: {
    outcome: { type: 'string', enum: ['planned', 'blocked'] },
    slug: str,
    tickets: strs,
    assumptions: strs,
    questions: strs,
    notes: str,
  },
  required: ['outcome', 'slug', 'tickets', 'assumptions'],
}
const BRANCH = { type: 'object', properties: { ok: { type: 'boolean' }, problem: str }, required: ['ok'] }
const BOARD = {
  type: 'object',
  properties: {
    head: { type: 'string', description: 'git rev-parse HEAD' },
    tickets: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          path: str,
          title: str,
          status: str,
          unblocked: { type: 'boolean', description: 'every ticket in its Blocked by line is done' },
        },
        required: ['path', 'title', 'status', 'unblocked'],
      },
    },
    manualChecks: strs,
  },
  required: ['head', 'tickets', 'manualChecks'],
}
const IMPL = {
  type: 'object',
  properties: {
    outcome: { type: 'string', enum: ['done', 'handoff', 'needs-human'] },
    commits: strs,
    notes: str,
  },
  required: ['outcome', 'commits', 'notes'],
}
const REVIEW = {
  type: 'object',
  properties: {
    findings: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          severity: { type: 'string', enum: ['must-fix', 'should-fix', 'nit'] },
          where: { type: 'string', description: 'file:line, or spec/ticket section' },
          problem: str,
          fix: str,
        },
        required: ['severity', 'where', 'problem', 'fix'],
      },
    },
  },
  required: ['findings'],
}

const TRIAGE = {
  type: 'object',
  properties: {
    head: { type: 'string', description: 'git rev-parse HEAD' },
    decisions: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          id: { type: 'integer', description: 'the finding number' },
          apply: { type: 'boolean' },
          reason: { type: 'string', description: 'one line' },
        },
        required: ['id', 'apply', 'reason'],
      },
    },
    followUps: { type: 'array', items: str, description: 'worthwhile work beyond the spec, one line each' },
  },
  required: ['head', 'decisions', 'followUps'],
}

const format = (findings) =>
  findings.map((f) => `- [${f.severity}] ${f.where}: ${f.problem} Fix: ${f.fix}`).join('\n')
const listing = (findings) =>
  findings
    .map((f) => `- [${f.severity}] (${f.source}) ${f.where}: ${f.problem} Fix: ${f.fix}\n  Triage: ${f.reason}`)
    .join('\n')

// ── Setup ────────────────────────────────────────────────────────────────────
phase('Setup')
const setup = await agent(
  `Check the repo before a feature run. The run's input is:\n\n> ${input}\n
- The working tree must be clean apart from files under \`.scratch/\`; otherwise return ok=false naming the dirty files.
- If the input is the slug of a folder \`.scratch/<slug>/\` that holds \`spec.md\` and \`issues/\`, return that slug as existingPlan; otherwise return it empty.
Change nothing.`,
  { label: 'setup', phase: 'Setup', schema: SETUP, ...CHORE },
)
if (!setup?.ok) return { stopped: setup?.problem ?? 'setup agent failed' }

// ── Plan ─────────────────────────────────────────────────────────────────────
let feature = setup.existingPlan
let plan = null
if (!feature) {
  phase('Plan')
  plan = await agent(`Plan this feature:\n\n> ${input}`, {
    label: 'plan',
    phase: 'Plan',
    agentType: 'planner',
    schema: PLANNED,
    ...PLAN,
  })
  if (!plan) return { stopped: 'planner failed' }
  if (plan.outcome === 'blocked') return { stopped: 'planner needs answers', questions: plan.questions, plan }
  feature = plan.slug

  const critique = await agent(
    `Critique the feature plan in \`.scratch/${feature}/\` (spec.md and issues/) before medium-effort agents implement it ticket by ticket. The brief was:\n\n> ${input}\n
Read the code it builds on. Judge it against the "Quality bar" in \`.claude/agents/planner.md\`, and also: does it deliver the brief; is every contract that more than one ticket depends on settled in the spec; are tickets sized to fit one fresh context; are the blocking edges right; what in the design only compensates for how the code is built today (a flag, an object shared to outlive a widget, a timing workaround), and would a prefactor remove it; does the Fit verdict hold; do Manual checks cover every timing or feel assumption? Plan detail that one ticket owns (signatures, DDL, constants, file lists, private helpers, exact test sequences, routes and payloads only that ticket uses) is a must-fix: name what to cut. must-fix = the plan would produce the wrong thing, stall an implementer, or decide what one ticket owns. Read only; change nothing.`,
    { label: 'critique plan', phase: 'Plan', schema: REVIEW, ...CRITIC },
  )
  const actionable = (critique?.findings ?? []).filter((f) => f.severity !== 'nit')
  if (actionable.length) {
    const revised = await agent(
      `Revise the feature plan in \`.scratch/${feature}/\` (brief: "${input}") to address these critique findings:\n${format(actionable)}`,
      { label: 'revise plan', phase: 'Plan', agentType: 'planner', schema: PLANNED, ...PLAN },
    )
    if (revised?.outcome === 'blocked') {
      return { stopped: 'planner needs answers', questions: revised.questions, plan: revised }
    }
    if (revised) plan = { ...revised, slug: feature, critique: `${actionable.length} findings addressed` }
  }
}

const dir = `.scratch/${feature}`
const branch = `feature/${feature}`
const branched = await agent(
  `Check out \`${branch}\`, creating it from \`main\` if it doesn't exist. Then, if \`${dir}/\` has uncommitted changes, commit them as "Plan ${feature}". Return ok=false with the reason if anything fails.`,
  { label: 'branch', phase: 'Setup', schema: BRANCH, ...CHORE },
)
if (!branched?.ok) return { feature, plan, stopped: branched?.problem ?? 'branch agent failed' }

// ── Tickets ──────────────────────────────────────────────────────────────────
async function shipTicket(ticket, base) {
  const p = { phase: 'Tickets', agentType: 'implementer', schema: IMPL, ...WORK }
  let impl = await agent(`Implement the ticket at \`${ticket.path}\`.`, {
    ...p,
    label: `implement: ${ticket.title}`,
  })
  for (let n = 1; impl?.outcome === 'handoff' && n <= MAX_HANDOFFS; n++) {
    log(`${ticket.title}: handoff ${n}, starting a fresh implementer`)
    impl = await agent(`Continue the ticket at \`${ticket.path}\` from its \`## Handoff\` section.`, {
      ...p,
      label: `continue (${n}): ${ticket.title}`,
    })
  }
  if (impl?.outcome !== 'done') {
    return { ticket: ticket.title, outcome: impl?.outcome ?? 'agent-failed', notes: impl?.notes }
  }
  const notes = [impl.notes] // surprises and follow-ups: the reviewer checks them, triage judges them

  // Findings no fix round was asked to apply (nits, and should-fix after round 1). They go to
  // triage after the final review instead of being dropped.
  const leftover = []
  for (let round = 1; ; round++) {
    const review = await agent(`Review the ticket at \`${ticket.path}\`; its commits are \`${base}..HEAD\`. The implementer's notes:\n\n> ${notes.join(' ')}\n\nTreat any surprise or follow-up in them as a lead: a problem fixed in one place may also reach the shipped app by another path.`, {
      label: `review ${round}: ${ticket.title}`,
      phase: 'Tickets',
      agentType: 'reviewer',
      schema: REVIEW,
      ...WORK,
    })
    if (!review) return { ticket: ticket.title, outcome: 'review-failed', notes: notes.join(' ') }
    // Round 1 also sends should-fix findings back; later rounds gate only on must-fix.
    const actionable = review.findings.filter(
      (f) => f.severity === 'must-fix' || (round === 1 && f.severity === 'should-fix'),
    )
    leftover.push(...review.findings.filter((f) => !actionable.includes(f)))
    if (!actionable.length) {
      return { ticket: ticket.title, outcome: 'done', notes: notes.join(' '), leftover }
    }
    if (round > MAX_FIX_ROUNDS) {
      return { ticket: ticket.title, outcome: 'unresolved-review', open: format(actionable) }
    }
    const fix = await agent(`Apply these review findings to the ticket at \`${ticket.path}\`:\n${format(actionable)}`, {
      ...p,
      label: `fix ${round}: ${ticket.title}`,
    })
    if (fix?.outcome !== 'done') {
      return { ticket: ticket.title, outcome: fix?.outcome ?? 'agent-failed', open: format(actionable) }
    }
    notes.push(fix.notes)
  }
}

phase('Tickets')
const attempted = new Set() // one attempt per ticket per run, so a stuck ticket can't loop
const results = []
let open = []
let manualChecks = [] // what offscreen tests can't judge: the final report asks the user to try these
for (;;) {
  const board = await agent(
    `Read every ticket file in \`${dir}/issues/\`. For each, return its path, title, status line, and whether it is unblocked: every ticket named in its "Blocked by" line has status \`resolved\`. Also return \`git rev-parse HEAD\`, and each bullet of the \`## Manual checks\` section of \`${dir}/spec.md\` word for word as manualChecks (empty when the section is missing or says None). Read only; change nothing.`,
    { label: 'scout', phase: 'Tickets', schema: BOARD, ...CHORE },
  )
  if (!board) return { feature, branch, plan, stopped: 'scout agent failed', results }
  manualChecks = board.manualChecks ?? []
  // The scout may return the whole line (`Status: resolved`); keep only the value.
  for (const t of board.tickets) t.status = t.status.replace(/^\W*status\W*/i, '').trim()
  const fresh = board.tickets.filter((t) => !attempted.has(t.path))
  // A claimed ticket was left by an earlier, interrupted run: finish it first.
  const next =
    fresh.find((t) => t.status === 'claimed') ??
    fresh.find((t) => t.status === 'ready-for-agent' && t.unblocked)
  if (!next) {
    open = board.tickets.filter((t) => t.status !== 'resolved').map((t) => `${t.title} (${t.status})`)
    break
  }
  attempted.add(next.path)
  const result = await shipTicket(next, board.head)
  log(`${result.ticket}: ${result.outcome}`)
  results.push(result)
}

// ── Final review ─────────────────────────────────────────────────────────────
// Severity labels decide nothing here. One triage agent judges every open finding (the branch
// review's, plus each ticket's leftovers) against the current code; one implementer applies the
// accepted ones; one review checks those commits and gates on must-fix with at most one repair.
// No loop: reviewing fixes can produce findings forever.
async function triageAndFix(candidates) {
  if (!candidates.length) return { outcome: 'clean' }
  const numbered = candidates
    .map((f, i) => `${i + 1}. [${f.severity}] (${f.source}) ${f.where}: ${f.problem} Fix: ${f.fix}`)
    .join('\n')
  const triage = await agent(
    `Triage the findings still open after building the feature in \`${dir}/\` (spec.md, issues/). Some come from the review of the whole branch; others are what per-ticket reviews reported but no fix round applied; a [note] is an implementer's own remark on finishing a ticket, and a remark that reports nothing to do is a skip. Each is numbered, with its source in parentheses:\n\n${numbered}\n
For each finding, read the code it points at as it stands now and decide whether to apply it.
- Apply: behaviour a user could hit (a crash, a wrong result, valid input rejected), a spec requirement the code misses, a test gap that would let a regression in shipped behaviour pass unnoticed, or duplication or inconsistency the repo conventions call out that a small, local change removes. Apply a must-fix unless you confirm it is wrong.
- Skip: already fixed, wrong, a duplicate (name the other number), cosmetic, comment-only, an extra assertion on behaviour other tests already pin, a matter of taste, or a fix that costs more than the problem. Work beyond the spec's scope is skipped and named in followUps.
The severity label is a hint, not the decision: a nit can be a real bug, and a should-fix can be noise.
Give every number a decision with a one-line reason. Also return \`git rev-parse HEAD\`. Read only; change nothing.`,
    { label: 'triage findings', phase: 'Final review', schema: TRIAGE, ...JUDGE },
  )
  if (!triage) return { outcome: 'triage agent failed', open: format(candidates) }

  const byId = new Map(triage.decisions.map((d) => [d.id, d]))
  const decided = candidates.map((f, i) => {
    const d = byId.get(i + 1)
    return { ...f, apply: d?.apply ?? false, reason: d?.reason ?? 'not triaged' }
  })
  const apply = decided.filter((f) => f.apply)
  const skipped = decided.filter((f) => !f.apply)
  log(`Triage: applying ${apply.length} of ${decided.length} open findings, skipping ${skipped.length}`)
  const report = { applied: listing(apply), skipped: listing(skipped), followUps: triage.followUps }
  if (!apply.length) return { outcome: 'nothing worth applying', ...report }

  const p = { phase: 'Final review', agentType: 'implementer', schema: IMPL, ...WORK }
  const fix = await agent(
    `Whole-feature fix for the spec at \`${dir}/spec.md\`. Triage judged these findings worth applying; the Triage line under each says why. Apply all of them and nothing else, each as small as its finding asks:\n${listing(apply)}`,
    { ...p, label: 'fix: triaged findings' },
  )
  if (fix?.outcome !== 'done') return { outcome: `fix ${fix?.outcome ?? 'agent-failed'}`, notes: fix?.notes, ...report }

  const check = await agent(
    `Review the commits \`${triage.head}..HEAD\`. There is no ticket: they apply these triaged findings to the feature in \`${dir}/spec.md\`:\n${listing(apply)}\nCheck each change does what its finding asked, without breaking the spec or the repo conventions. must-fix = a change is wrong, incomplete, untested, or breaks something; everything else is a nit.`,
    { label: 'review: triaged fixes', phase: 'Final review', agentType: 'reviewer', schema: REVIEW, ...WORK },
  )
  if (!check) return { outcome: 'fixed; the check review failed', ...report }
  const broken = check.findings.filter((f) => f.severity === 'must-fix')
  if (!broken.length) return { outcome: 'fixed', ...report }
  const repair = await agent(
    `Whole-feature fix for the spec at \`${dir}/spec.md\`. Apply these findings from the review of the commits \`${triage.head}..HEAD\`:\n${format(broken)}`,
    { ...p, label: 'repair: triaged fixes' },
  )
  return {
    outcome: repair?.outcome === 'done' ? 'fixed after one repair' : `repair ${repair?.outcome ?? 'agent-failed'}`,
    ...(repair?.outcome === 'done' ? {} : { open: format(broken) }),
    ...report,
  }
}

let finalReview = { outcome: 'skipped: no ticket finished this run' }
if (results.some((r) => r.outcome === 'done')) {
  phase('Final review')
  const review = await agent(
    `Call the Skill tool with \`code-review\`. The fixed point is \`main\`; the spec is \`${dir}/spec.md\`. Return its findings, classifying each as must-fix (wrong behaviour, missed requirement), should-fix, or nit.`,
    { label: 'code-review', phase: 'Final review', schema: REVIEW, ...WORK },
  )
  // Exact repeats collapse here; near-duplicates are left to triage, which names them.
  const seen = new Set()
  const candidates = [
    ...(review?.findings ?? []).map((f) => ({ ...f, source: 'branch review' })),
    ...results.flatMap((r) => (r.leftover ?? []).map((f) => ({ ...f, source: r.ticket }))),
    ...results
      .filter((r) => r.notes)
      .map((r) => ({
        severity: 'note',
        where: 'implementer notes',
        problem: r.notes,
        fix: 'Judge whether the note names a problem that still needs work.',
        source: r.ticket,
      })),
  ].filter((f) => {
    const key = `${f.where}\n${f.problem}`
    return !seen.has(key) && seen.add(key)
  })
  finalReview = { review: review ? `${review.findings.length} findings` : 'code-review agent failed', ...(await triageAndFix(candidates)) }
  if (finalReview.followUps?.length) {
    const bullets = finalReview.followUps.map((f) => `- ${f}`).join('\n')
    await agent(
      `Append these bullets to \`${dir}/follow-ups.md\`, creating it with the heading "# Follow-ups" if it doesn't exist, and commit that file alone as "Record ${feature} follow-ups":\n\n${bullets}`,
      { label: 'record follow-ups', phase: 'Final review', ...CHORE },
    )
  }
}

return {
  feature,
  branch,
  plan,
  results: results.map(({ leftover, ...r }) => ({ ...r, ...(leftover ? { leftover: leftover.length } : {}) })),
  open,
  finalReview,
  manualChecks,
}
