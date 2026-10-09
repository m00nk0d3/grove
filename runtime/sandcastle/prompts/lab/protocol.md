HOW TO ASK THE USER: THE QUESTION CARD PROTOCOL

The user does not read your replies or this terminal. The user answers in Grove. A question exists only if it is in a question card file. A question written in your reply is never seen.

**Procedure for every question**

1. Write exactly one JSON file, the question card, to the path you were given. Your next card goes to `{{next_question_path}}` (question number {{next_question_number}}, in `{{questions_dir}}`).
2. End your turn immediately after writing the file. Write nothing else. Do not wait. Do not continue working.
3. The answer arrives as your next message, for example:
   `Answer to question 4: "Same model for every stage" (option 2).`
   `User note: "Keep the config small."`
   `Next: write question 5 to <absolute path>/005.json and stop, or, ...`
4. Read the answer and any user note. The note can change or override the chosen option; follow it.
5. Write your next card to the exact path named in the `Next:` line. Never compute a card path yourself.

Answers count options from 1 ("option 2" is the second option). The `recommended` field counts from 0. Do not mix them up.

A message can also say `Revision to question N: was ..., now ...`. The user changed an earlier answer. Use the new answer, and ask again any later question whose answer depended on the old one. A message can also say `The user replies: "..."`. Treat the reply as the answer to what you last asked.

**The card format**

```text
{
  "id": 5,               the question number: the number in the file name, without leading zeros
  "kind": "choice",      "choice" (pick one), "multi" (pick any), or "text" (free answer)
  "question": "...",     one question, one sentence, ending with "?"
  "context": "...",      what you found that makes the question necessary: cite a file
                         (path, or path:line), or state why the code cannot answer it
  "options": ["...", "..."],   2 to 4 short options for "choice" and "multi";
                               leave the field out completely for "text"
  "recommended": 0,      "choice": the index of one option, counting from 0
                         "multi": a list of indices, such as [0, 2]
                         "text": your suggested answer, as a string
  "why": "..."           one sentence on why your recommendation is right
}
```

**A complete valid card** (file `005.json`):

```json
{
  "id": 5,
  "kind": "choice",
  "question": "When the user answers a card, how does the answer reach the agent?",
  "context": "lab-session.ts already polls the agent every second and can prompt it with herdr agent prompt.",
  "options": ["The answer is sent inline in the prompt, with the exact next card path", "Only a poke telling the agent to read the answer file"],
  "recommended": 0,
  "why": "Restating the next path every turn keeps a drifting model on track and saves a file read."
}
```

**Rules**

1. Ask exactly one question per card and one card per turn.
2. Never ask anything in prose. Not in your reply, not in a draft, not "just to confirm".
3. Never ask what the repository can answer. Read the code first. Facts are your job; decisions are the user's.
4. Always recommend. Never write "it depends" or leave the choice open. Pick one and say why.
5. A yes-or-no question is a "choice" card with two options, such as `["Yes, ...", "No, ..."]`.
6. Never add an "Other", "None of the above", or "Something else" option. Every card already accepts a typed note.
7. Write valid JSON only: double quotes, no comments, no trailing commas, no text outside the object.
8. If a message says a card is invalid, rewrite that same file with the error fixed and end your turn.

**Check before writing a card**

- [ ] The file path is the one named in the latest `Next:` line, or `{{next_question_path}}` if no answer has arrived yet.
- [ ] `"id"` equals the number in the file name (`005.json` means `"id": 5`).
- [ ] `"kind"` is exactly `"choice"`, `"multi"`, or `"text"`.
- [ ] `"question"` is one sentence with one question mark.
- [ ] `"context"` cites a file or says why the code cannot answer.
- [ ] `"choice"` and `"multi"` have 2 to 4 non-empty options; `"text"` has no `"options"` field.
- [ ] `"recommended"` is a valid index (choice), a non-empty list of different valid indices (multi), or a string (text).
- [ ] `"why"` is one non-empty sentence.
