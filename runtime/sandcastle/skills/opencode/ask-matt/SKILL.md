---
name: ask-matt
description: Router that helps the user find and install agent skills when they ask questions like "how do I do X", "find a skill for X", or express interest in extending capabilities.
---

When you see this skill triggered, it means the user is asking about an available or installable agent capability. Your job is to:

1. **Search** the skills directory (`runtime/sandcastle/skills/`) for relevant files
2. **Read** the SKILL.md for each candidate skill to understand what it does
3. **Present** a concise summary of matching skills with their capabilities and how they relate to the user's request
4. **Offer** to launch the relevant skill or explain more about what's available

The skills directory structure is:
```
runtime/sandcastle/skills/
├── ask-matt/SKILL.md
├── chief-of-staff/SKILL.md
├── claude-handoff/SKILL.md
├── code-review/SKILL.md
├── domain-modeling/SKILL.md
└── ... (all available skills)
```

Each skill has a single SKILL.md file that describes when to use it, what it does, and any important notes.
