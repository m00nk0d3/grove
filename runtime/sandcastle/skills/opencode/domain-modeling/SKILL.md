---
name: domain-modeling
description: Build and sharpen the project's domain model. Use when designing or improving a module's interface, finding deepening opportunities, making code more testable or AI-navigable, or when another skill needs the deep-module vocabulary.
---

Build a precise mental model of how your codebase works, then document it so others (and future you) can navigate it without guessing. The goal is to create terminology that's specific enough to be useful but general enough to not overfit to any single implementation detail.

Key principles:
1. **Name things by what they do, not how they're implemented**. A `UserRepository` isn't just a database wrapper — it's where user data lives and comes from. That's the model, regardless of whether it's SQL, MongoDB, or an in-memory map.

2. **Start with boundaries**. Every domain needs clear edges: what counts as "in scope" vs "out of scope", what can be changed independently vs must change together, and where responsibilities end.

3. **Name the seams**. Where do components connect? What contracts exist between them? These are your integration points — and they're where bugs hide. Make them explicit.

4. **One responsibility per boundary**. If a module does two fundamentally different things (e.g., "manages users" AND "sends notifications"), it has two responsibilities. The model should reflect that split, even if the code doesn't yet.

5. **Document the "why", not just the "what"**. A diagram saying "User → Service → DB" is less useful than explaining why those three things exist separately: "Users are a bounded context; Services mediate between User and DB because they need validation that neither knows about; DB handles persistence."

6. **Version your model**. Domain models change as the system evolves. Track versions so you can understand how the mental model has shifted, not just the code.

7. **Keep it lightweight**. The model should be readable in under a minute. If it's longer than that, something is wrong — you've gone from model to implementation manual.

Output:
- A GLOSSARY.md with key terms and their definitions (no more than 20-30 terms)
- An ADR-FORMAT.md documenting significant design decisions and the tradeoffs they involve
- Optional: a CONTEXT-MAP.md that shows how different bounded contexts relate to each other

The model should be written by reading code, not by guessing what "should" exist. If you see a pattern in the code, document it. If you don't see one, don't invent it — leave a gap for someone else to fill.
