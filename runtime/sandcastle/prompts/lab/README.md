# Lab stage prompts

Grove's own instructions for the agents of a Lab session, one file per stage.
See `docs/LAB_DESIGN.md`, "Agent instructions".

| File           | Stage                                                        |
|----------------|--------------------------------------------------------------|
| `scout.md`     | Scout: read the code the entry concerns and write `scout.md` |
| `interview.md` | Interview: question cards until coverage is complete         |
| `spec.md`      | Spec: `spec.md`, and `CONTEXT.md` and decision records       |
| `tickets.md`   | Tickets: `tickets.json`                                      |
| `shape.md`     | Shape: a bug report, `issue.md`, through question cards      |
| `protocol.md`  | The question card protocol, included as `{{protocol}}`       |

A repository can replace any of these with `.grove/lab/prompts/<file>` in its
base checkout.

## Shape

Every stage prompt has these sections, in this order, as `##` headings:
**ROLE**, **GOAL**, **INPUTS**, **PROCEDURE**, **RULES**, **EXAMPLES**,
**SELF-CHECK**, **WHEN DONE**. The prompts are written for small local models as
well as large hosted ones: numbered steps, imperative rules that can be
checked, exact output formats, good and bad examples with the reason each bad
one is bad.

## Placeholders

The session runtime replaces each `{{name}}` before sending a prompt. Every
placeholder is available in every stage; a value that does not exist is the
literal text `(none)`. A placeholder not listed here is left as written and
reported as an error by the tests.

| Placeholder                | Value                                                                 |
|----------------------------|-----------------------------------------------------------------------|
| `{{repo}}`                 | Absolute path of the base checkout                                    |
| `{{entry_kind}}`           | `idea` or `bug`                                                       |
| `{{entry_title}}`          | The entry's title: the first line of its text                         |
| `{{entry_text}}`           | The entry's captured text                                             |
| `{{stage}}`                | This stage's name: `scout`, `interview`, `spec`, `tickets`, or `shape` |
| `{{done_file}}`            | Absolute path the agent writes `{{stage}}` to when the stage is finished |
| `{{entry_dir}}`            | Absolute path of the entry's directory                                |
| `{{artifacts_dir}}`        | Absolute path of the entry's drafts directory                         |
| `{{repo_map}}`             | The repository map, built by Grove                                    |
| `{{scout_file}}`           | Absolute path of `scout.md`                                           |
| `{{scout_notes}}`          | Content of `scout.md`                                                 |
| `{{shaped_report}}`        | For a bug escalated to a grill, its shaped report                    |
| `{{questions_dir}}`        | Absolute path of the question cards directory                         |
| `{{next_question_number}}` | The number of the next question card                                  |
| `{{next_question_path}}`   | Absolute path of the next question card                               |
| `{{interview_so_far}}`     | Every question asked so far and its answer, one line each             |
| `{{coverage_file}}`        | Absolute path of `coverage.json`                                      |
| `{{coverage}}`             | Content of `coverage.json`                                            |
| `{{spec_file}}`            | Absolute path of the spec draft, `artifacts/spec.md`                  |
| `{{spec}}`                 | Content of the spec draft                                             |
| `{{tickets_file}}`         | Absolute path of `artifacts/tickets.json`                             |
| `{{issue_file}}`           | Absolute path of `artifacts/issue.md`                                 |
| `{{previous_draft}}`       | This stage's own output from an earlier session, to continue from     |
| `{{next_adr_number}}`      | Four digits: one past the highest decision record in the repository and the drafts |
| `{{protocol}}`             | `protocol.md`, rendered                                               |

## Outputs

| Stage     | Writes                                                                 |
|-----------|------------------------------------------------------------------------|
| Scout     | `{{scout_file}}`                                                       |
| Interview | question cards, `{{coverage_file}}`                                    |
| Spec      | `{{spec_file}}`; `CONTEXT.md` and `docs/adr/NNNN-<slug>.md` under `{{artifacts_dir}}` at their repository-relative paths |
| Tickets   | `{{tickets_file}}`                                                     |
| Shape     | question cards, `{{issue_file}}`                                       |

Each stage ends by writing `{{stage}}` to `{{done_file}}` and stopping. The
runtime validates the output and, when it is malformed, sends the agent a
prompt naming each problem.

`spec.md` sections, in order: `# <title>`, `## Problem`, `## Goals`,
`## Non-goals`, `## User stories`, `## Decisions`, `## Design`, `## Testing`,
`## Edge cases`, `## Open questions`.

`tickets.json`: `{"tickets": [{"key", "title", "body", "blocked_by"}]}`; keys
`"01"`, `"02"`, … in dependency order; each body has `## What`, `## Why`,
`## Where`, `## Acceptance criteria`, `## Out of scope`.

`issue.md` sections, in order: `# <title>`, `## Summary`,
`## Steps to reproduce`, `## Expected behaviour`, `## Actual behaviour`,
`## Environment`, `## Component`, `## Notes`.

`coverage.json`: an object with the keys `scope`, `triggers`, `data`,
`interface`, `errors`, `concurrency`, `compatibility`, `testing`, `rollout`;
each value is `"covered"`, `"open"`, or `"n/a: <reason>"`.

`scout.md` sections, in order: `# Scout notes`, `## Relevant files`,
`## How it works today`, `## Constraints`, `## Open questions`.

## Sources

The prompts are Grove's own. Their interview discipline follows the `grill-me`
skill; they also draw on Matt Pocock's skills (MIT), as `NOTICE` records.
