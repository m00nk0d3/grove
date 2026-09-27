import test from "node:test";
import assert from "node:assert/strict";
import {
  resolveProjectStatuses,
  setIssueProjectStatus,
} from "./project-board.js";

function itemsResponse(
  items: {
    id: string;
    projectId: string;
    title: string;
    fieldId?: string;
    options?: { id: string; name: string }[];
  }[],
): string {
  return JSON.stringify({
    data: {
      repository: {
        issue: {
          projectItems: {
            nodes: items.map((item) => ({
              id: item.id,
              project: {
                id: item.projectId,
                title: item.title,
                statusField: item.fieldId
                  ? { id: item.fieldId, options: item.options ?? [] }
                  : null,
              },
            })),
          },
        },
      },
    },
  });
}

const BOARD = {
  id: "item-1",
  projectId: "proj-1",
  title: "Grove board",
  fieldId: "field-1",
  options: [
    { id: "opt-todo", name: "Todo" },
    { id: "opt-progress", name: "In Progress" },
    { id: "opt-review", name: "In Review" },
  ],
};

test("project status sync defaults to progress then review, and can be disabled", () => {
  assert.deepEqual(resolveProjectStatuses({}), {
    start: "In Progress",
    publish: "In Review",
  });
  assert.deepEqual(
    resolveProjectStatuses({
      AGENT_FLOW_PROJECT_STATUS_START: "Doing",
      AGENT_FLOW_PROJECT_STATUS_PUBLISH: "QA",
    }),
    { start: "Doing", publish: "QA" },
  );
  assert.equal(resolveProjectStatuses({ AGENT_FLOW_PROJECT_STATUS_SYNC: "off" }), null);
  assert.equal(resolveProjectStatuses({ AGENT_FLOW_PROJECT_STATUS_SYNC: "0" }), null);
});

test("issue status is set through the matching board option", () => {
  const calls: { command: string; args: string[] }[] = [];
  const runner = (command: string, args: string[]): string => {
    calls.push({ command, args });
    if (args.some((arg) => arg.includes("projectItems"))) {
      return itemsResponse([BOARD]);
    }
    return JSON.stringify({ data: { updateProjectV2ItemFieldValue: {} } });
  };

  const updates = setIssueProjectStatus("owner/repo", "42", "In Progress", runner);
  assert.deepEqual(updates, [
    { projectTitle: "Grove board", itemId: "item-1", optionName: "In Progress" },
  ]);
  const mutation = calls.at(-1)!;
  assert.equal(mutation.command, "gh");
  const joined = mutation.args.join(" ");
  assert.match(joined, /item-1/);
  assert.match(joined, /field-1/);
  assert.match(joined, /opt-progress/);
  assert.match(joined, /proj-1/);
});

test("option matching ignores case differences in board wording", () => {
  const runner = (command: string, args: string[]): string => {
    if (args.some((arg) => arg.includes("projectItems"))) {
      return itemsResponse([BOARD]);
    }
    return JSON.stringify({ data: { updateProjectV2ItemFieldValue: {} } });
  };

  const updates = setIssueProjectStatus("owner/repo", "42", "in progress", runner);
  assert.equal(updates[0].optionName, "In Progress");
});

test("every board with a Status field is updated", () => {
  const mutated: string[] = [];
  const runner = (command: string, args: string[]): string => {
    if (args.some((arg) => arg.includes("projectItems"))) {
      return itemsResponse([
        BOARD,
        {
          id: "item-2",
          projectId: "proj-2",
          title: "Second board",
          fieldId: "field-2",
          options: [{ id: "opt-2", name: "In Progress" }],
        },
        { id: "item-3", projectId: "proj-3", title: "No status field" },
      ]);
    }
    mutated.push(args.join(" "));
    return JSON.stringify({ data: { updateProjectV2ItemFieldValue: {} } });
  };

  const updates = setIssueProjectStatus("owner/repo", "42", "In Progress", runner);
  assert.deepEqual(
    updates.map((update) => update.projectTitle),
    ["Grove board", "Second board"],
  );
  assert.equal(mutated.length, 2);
});

test("missing boards, options, and API failures are reported, not hidden", () => {
  const ok = (command: string, args: string[]): string => {
    if (args.some((arg) => arg.includes("projectItems"))) {
      return itemsResponse([BOARD]);
    }
    return JSON.stringify({ data: { updateProjectV2ItemFieldValue: {} } });
  };
  assert.throws(
    () => setIssueProjectStatus("owner/repo", "42", "Done", ok),
    /has no "Done" option/,
  );
  assert.throws(
    () =>
      setIssueProjectStatus("owner/repo", "42", "In Progress", (command, args) =>
        args.some((arg) => arg.includes("projectItems"))
          ? itemsResponse([{ id: "i", projectId: "p", title: "Plain board" }])
          : "{}",
      ),
    /no project board with a Status field/,
  );
  assert.throws(
    () =>
      setIssueProjectStatus("owner/repo", "not-a-number", "In Progress", ok),
    /Invalid repository or issue/,
  );
  assert.throws(
    () =>
      setIssueProjectStatus("owner/repo", "42", "In Progress", () => {
        throw new Error("gh exited with status 1: Not Found");
      }),
    /GitHub API request failed/,
  );
  assert.throws(
    () =>
      setIssueProjectStatus(
        "owner/repo",
        "42",
        "In Progress",
        () => "this is not json",
      ),
    /invalid JSON/,
  );
});
