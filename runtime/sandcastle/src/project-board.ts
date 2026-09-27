import { runCommand, type CommandRunner } from "./workflow-utils.js";

export interface ProjectStatusUpdate {
  projectTitle: string;
  itemId: string;
  optionName: string;
}

export interface ProjectStatuses {
  start: string;
  publish: string;
}

// resolveProjectStatuses reads the board-sync configuration. Returns null when
// the user disabled it; otherwise the status names to apply when a workflow
// starts and when its pull request is published. Names default to the common
// board vocabulary and are matched case-insensitively against each board's own
// Status options, so a board that says "In progress" still matches.
export function resolveProjectStatuses(
  env: NodeJS.ProcessEnv = process.env,
): ProjectStatuses | null {
  const sync = (env.AGENT_FLOW_PROJECT_STATUS_SYNC ?? "on").trim().toLowerCase();
  if (["0", "off", "false", "no"].includes(sync)) return null;
  return {
    start: env.AGENT_FLOW_PROJECT_STATUS_START?.trim() || "In Progress",
    publish: env.AGENT_FLOW_PROJECT_STATUS_PUBLISH?.trim() || "In Review",
  };
}

interface StatusOption {
  id: string;
  name: string;
}

interface ProjectItem {
  id: string;
  projectId: string;
  projectTitle: string;
  fieldId: string | null;
  options: StatusOption[];
}

const ITEMS_QUERY = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    issue(number: $number) {
      projectItems(first: 10) {
        nodes {
          id
          project {
            id
            title
            statusField: field(name: "Status") {
              ... on ProjectV2SingleSelectField {
                id
                options { id name }
              }
            }
          }
        }
      }
    }
  }
}`;

const UPDATE_MUTATION = `mutation($projectId: ID!, $itemId: ID!, $fieldId: ID!, $optionId: String!) {
  updateProjectV2ItemFieldValue(input: {
    projectId: $projectId,
    itemId: $itemId,
    fieldId: $fieldId,
    value: { singleSelectOptionId: $optionId }
  }) {
    projectV2Item { id }
  }
}`;

function graphql(
  runner: CommandRunner,
  query: string,
  variables: Record<string, string>,
): unknown {
  const args = ["api", "graphql", "-f", `query=${query}`];
  for (const [key, value] of Object.entries(variables)) {
    args.push("-F", `${key}=${value}`);
  }
  let raw: string;
  try {
    raw = runner("gh", args);
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error);
    throw new Error(`GitHub API request failed: ${detail}`);
  }
  try {
    return JSON.parse(raw);
  } catch {
    throw new Error(`GitHub returned invalid JSON: ${raw.slice(0, 200)}`);
  }
}

// setIssueProjectStatus moves every project board the issue sits on to the
// named Status option. It throws when the issue is on no board with a Status
// field, when no option matches, or when the API call fails — callers treat
// this as best-effort and log a warning rather than failing the workflow.
export function setIssueProjectStatus(
  repo: string,
  issueNum: string,
  statusName: string,
  runner: CommandRunner = runCommand,
): ProjectStatusUpdate[] {
  const [owner, name] = repo.split("/");
  const number = Number(issueNum);
  if (!owner || !name || !Number.isInteger(number) || number <= 0) {
    throw new Error(`Invalid repository or issue: ${repo}#${issueNum}`);
  }
  const response = graphql(runner, ITEMS_QUERY, {
    owner,
    name,
    number: String(number),
  }) as {
    data?: {
      repository?: {
        issue?: {
          projectItems?: {
            nodes?: {
              id?: string;
              project?: {
                id?: string;
                title?: string;
                statusField?: { id?: string; options?: StatusOption[] };
              };
            }[];
          };
        };
      };
    };
  };
  const nodes = response.data?.repository?.issue?.projectItems?.nodes ?? [];
  const items: ProjectItem[] = nodes.map((node) => ({
    id: node.id ?? "",
    projectId: node.project?.id ?? "",
    projectTitle: node.project?.title ?? "unknown project",
    fieldId: node.project?.statusField?.id ?? null,
    options: node.project?.statusField?.options ?? [],
  }));
  const withStatus = items.filter(
    (item) => item.id && item.projectId && item.fieldId,
  );
  if (withStatus.length === 0) {
    throw new Error(
      `Issue ${repo}#${issueNum} is on no project board with a Status field.`,
    );
  }
  const wanted = statusName.toLowerCase();
  const updates: ProjectStatusUpdate[] = [];
  for (const item of withStatus) {
    const option = item.options.find(
      (candidate) => candidate.name.toLowerCase() === wanted,
    );
    if (!option) {
      const available = item.options.map((candidate) => candidate.name).join(", ");
      throw new Error(
        `Project "${item.projectTitle}" has no "${statusName}" option (has: ${available || "none"}).`,
      );
    }
    graphql(runner, UPDATE_MUTATION, {
      projectId: item.projectId,
      itemId: item.id,
      fieldId: item.fieldId as string,
      optionId: option.id,
    });
    updates.push({
      projectTitle: item.projectTitle,
      itemId: item.id,
      optionName: option.name,
    });
  }
  return updates;
}
