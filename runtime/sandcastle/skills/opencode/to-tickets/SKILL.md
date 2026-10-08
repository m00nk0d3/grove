---
name: to-tickets
description: Break a plan, spec, or the current conversation into a set of tracer-bullet tickets, each declaring its blocking edges. Published to the configured tracker (edges as text in one file per ticket locally, or native blocking links on a real tracker).
---

Break down the work defined in the spec (or discussion) into the smallest actionable units possible. Each ticket should be small enough that it can be reviewed and completed independently, but large enough to have clear acceptance criteria and not require excessive coordination.

Ticket structure:
- **key**: A unique identifier ("01", "02", etc.) in dependency order (blockers first)
- **title**: A concise description of what this ticket accomplishes
- **body**: The detailed work description, including:
  - What exactly needs to be built/changed
  - Acceptance criteria or test cases
  - Any dependencies on other tickets (which keys this blocks and is blocked by)
- **blocked_by**: Array of ticket keys that must complete before this one can start

General guidelines:
1. **One atomic unit per ticket** if possible. If a task has multiple distinct concerns, split it. A ticket that touches three different areas of the codebase is likely too big.

2. **Order matters**. List blockers first (tickets with no dependencies come first). This makes the dependency graph explicit and helps identify parallelizable work.

3. **Include enough detail for a reviewer to understand** what needs to be done, but don't write implementation details in the ticket body. The ticket is about *what*, not *how*.

4. **Make blockers explicit**. If ticket A depends on ticket B, list B in A's blocked_by array. This prevents the "why can't I do this?" problem where two people are working on things that block each other.

5. **If something doesn't fit a ticket**, leave it out or add it as a follow-up. Don't force ambiguous work into tickets — better to have a gap than a wrong ticket.

6. **Keep the list manageable**. If you have more than 20-30 items, reconsider whether they're too small and can be grouped. The goal is actionable units, not atomic operations.

Output:
- A JSON file with an array of tickets, formatted as shown below
```json
{
  "tickets": [
    {
      "key": "01",
      "title": "Add user authentication flow",
      "body": "...",
      "blocked_by": []
    },
    {
      "key": "02",
      "title": "Update API documentation to reflect new auth endpoint",
      "body": "...",
      "blocked_by": ["01"]
    }
  ]
}
```

Progress tracking: overwrite the stage file with "tickets" when you finish writing the ticket breakdown. The session is complete once tickets are written and ready for review (or once the user confirms no more work needs to be done).
