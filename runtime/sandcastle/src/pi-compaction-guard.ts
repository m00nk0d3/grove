import fs from "node:fs";

interface CompactionEvent {
  reason?: string;
  compactionEntry?: {
    tokensBefore?: number;
  };
  errorMessage?: string;
}

interface ExtensionApi {
  on(
    event: string,
    handler: (event: CompactionEvent) => void | Promise<void>,
  ): void;
  sendMessage(
    message: {
      customType: string;
      content: string;
      display: boolean;
    },
    options: {
      deliverAs: "steer";
    },
  ): void;
}

function appendAuditEvent(
  statusPath: string,
  event: Record<string, unknown>,
): void {
  fs.appendFileSync(
    statusPath,
    `${JSON.stringify({ timestamp: new Date().toISOString(), ...event })}\n`,
    "utf8",
  );
}

export function buildCompactionRecoveryMessage(assignment: string): string {
  return `Agent-flow compaction recovery:

Your conversation context was compacted. Treat the exact assignment and the
current filesystem/Git state as authoritative; do not rely only on the lossy
compaction summary.

Before continuing:
1. Re-read the exact original assignment below.
2. Inspect git status, the current diff, and every durable handoff or completion
   artifact named by the assignment.
3. Reconcile completed work, remaining acceptance criteria, validation results,
   and blockers from repository evidence.
4. Continue from the current state without repeating completed work or dropping
   requirements.

<exact-original-assignment>
${assignment}
</exact-original-assignment>`;
}

export default function compactionGuard(pi: ExtensionApi): void {
  const assignmentPath = process.env.AGENT_FLOW_ASSIGNMENT_FILE;
  const statusPath = process.env.AGENT_FLOW_COMPACTION_STATUS_FILE;
  if (!assignmentPath || !statusPath) {
    throw new Error(
      "The agent-flow compaction guard requires assignment and status paths.",
    );
  }

  const assignment = fs.readFileSync(assignmentPath, "utf8");
  appendAuditEvent(statusPath, { type: "guard-ready" });

  pi.on("agent_start", () => {
    appendAuditEvent(statusPath, { type: "agent-started" });
  });

  pi.on("agent_settled", () => {
    appendAuditEvent(statusPath, { type: "agent-settled" });
  });

  pi.on("session_before_compact", (event) => {
    appendAuditEvent(statusPath, {
      type: "compaction-started",
      reason: event.reason ?? "unknown",
    });
  });

  pi.on("session_compact", (event) => {
    appendAuditEvent(statusPath, {
      type: "compaction-completed",
      reason: event.reason ?? "unknown",
      tokensBefore: event.compactionEntry?.tokensBefore ?? null,
    });
    pi.sendMessage(
      {
        customType: "agent-flow-compaction-recovery",
        content: buildCompactionRecoveryMessage(assignment),
        display: true,
      },
      { deliverAs: "steer" },
    );
  });

  pi.on("session_compact_failed", (event) => {
    appendAuditEvent(statusPath, {
      type: "compaction-failed",
      reason: event.reason ?? "unknown",
      error: event.errorMessage ?? "Compaction aborted or failed",
    });
  });
}
