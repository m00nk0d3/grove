---
name: to-spec
description: Turn the current conversation into a spec and publish it as a draft specification. No interview — just synthesis of what you've already discussed. Use when you have enough information from context or previous work to produce a complete spec.
---

You've gathered enough evidence (from issue descriptions, code changes, user interviews, or prior drafts) that you can now write a clear specification for the change. This is not an interview — it's synthesis of what already exists.

What to include:
1. **Problem/Context**: What problem does this solve? Why does it matter? Reference the issue(s), PRs, or user needs that lead here.

2. **Solution overview**: What will be changed and why. Keep this high-level — save details for step 3.

3. **Detailed design**:
   - Architecture: what components are involved and how they interact
   - Data changes: any new types, tables, or fields
   - API/Interface changes: signatures, payloads, error codes
   - Code structure: files that will be created, modified, or deleted
   - Any architectural decisions made (and why)

4. **Acceptance criteria**: concrete, testable statements about what "done" looks like. Prefer examples over descriptions when possible.

5. **Known limitations and risks**: anything the change can't handle, or edge cases that are out of scope for this iteration. Don't leave these blank — it's better to be explicit about gaps than to pretend they don't exist.

6. **Dependencies and blockers**: any external work needed before this spec is safe to implement (e.g., "requires the new auth flow to land first").

7. **Implementation approach** (optional but helpful): rough sketch of how you'd implement it, prioritizing clarity over completeness. This helps reviewers understand what's feasible vs. aspirational.

Output:
- A single Markdown document that can serve as both a design doc and an implementation guide
- Use clear headings, bullet points, and code examples where they clarify the intent
- Keep it to one page if possible; if it grows longer, add a summary at the top

Boundaries:
- Don't invent requirements you haven't seen evidence for. If something isn't in the issue or clearly implied by context, call it out as an assumption and note what would need to change if it's wrong.
- Don't include implementation details that are still uncertain. Mark them as "TODO" or "pending decision".

Progress tracking: overwrite the stage file with "spec" when you finish writing the spec document. The next phase (tickets) will reference this document.
