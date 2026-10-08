---
name: grill-with-docs
description: Stress-test a design or decision with the user through structured questioning, building context and specifications as we go. Use when you need to validate assumptions, map tradeoffs, and produce both a spec and tickets from an idea.
---

Work through three phases in order: Interview → Spec → Tickets. Each phase produces concrete output before moving on. The session ends once all three are complete.

Phase 1: Interview (grilling)

Interview the user relentlessly until you reach shared understanding. Map this as a design tree: every decision branches into the decisions that depend on it. Work in rounds — ask all open questions in one go, collect answers, then recompute what's now known and what's still unknown. Only advance to the next round when the current frontier is settled.

Rules:
- Find facts yourself (read files, check docs, run commands). Never make the user look things up they could find.
- Each question must have clear options or a specific answer you need. Number your questions and give your recommended answer so the user knows what to aim for.
- When you hit something that requires a decision, put it in the frontier. Don't assume answers — ask explicitly.

Output: no file yet. Just gather enough information to write the spec.

Phase 2: Spec (to-spec)

Write a detailed specification from the interview. The spec should include:
- What this change does and why
- How it works (architecture, data flow, key decisions)
- What's in scope and what's out of scope (with rationale for exclusions)
- Acceptance criteria — concrete, testable statements about expected behavior
- Known limitations or tradeoffs

Output: a single Markdown document with clear sections. Use the format shown below as your template.

Phase 3: Tickets (to-tickets)

Break the spec into actionable tickets that can be reviewed and implemented independently. Each ticket should have:
- A clear title describing what needs to happen
- A body with concrete steps or acceptance criteria
- Explicit blockers (only if there's a real dependency on another work item)

Output: a JSON file with an array of tickets, ordered by dependency. Each ticket has a unique key ("01", "02", etc.), title, body, and optional list of blocking ticket keys.

General rules for all phases:
- Never commit code or push to Git during this session.
- Only write files under the artifacts directory (not in the repository).
- When you reach a decision point that affects multiple downstream decisions, confirm it before proceeding — don't guess.
- If the user says "I'm not sure about X" or similar, ask clarifying questions rather than assuming.

Progress tracking: overwrite the stage file with one word per phase as you go:
  interview  when you start gathering information
  spec       when you finish writing the spec
  tickets    when you finish writing the ticket breakdown
  drafted    when both spec and tickets are written and ready for review

After writing "drafted", tell the user the drafts are ready to review in Grove's Lab (inspector → Artifacts), then wait. If the user asks for changes, revise the files and write "drafted" again.
