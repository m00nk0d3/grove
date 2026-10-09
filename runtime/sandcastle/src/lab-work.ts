// A Lab stage's working folder. The Lab lives in the git common directory,
// and Claude Code treats everything inside .git as sensitive: it refuses to
// write there however its permissions are set. Each stage's agent therefore
// writes to a folder of its own under Grove's state directory, outside any
// repository, and the runtime keeps that folder and the Lab entry in step.
// See docs/LAB_DESIGN.md, "Storage".

import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

/** The paths an agent writes to, wherever they are. */
export interface AgentPaths {
  entryDir: string;
  artifactsDir: string;
  questionsDir: string;
  scoutFile: string;
  coverageFile: string;
  doneFile: string;
}

export function agentPaths(root: string): AgentPaths {
  return {
    entryDir: root,
    artifactsDir: path.join(root, "artifacts"),
    questionsDir: path.join(root, "questions"),
    scoutFile: path.join(root, "scout.md"),
    coverageFile: path.join(root, "coverage.json"),
    doneFile: path.join(root, "done"),
  };
}

/** Grove's state directory: GROVE_STATE_DIR, or ~/.grove. */
export function groveStateDir(env: NodeJS.ProcessEnv = process.env): string {
  return env.GROVE_STATE_DIR || path.join(os.homedir(), ".grove");
}

/** Where one stage of one run works. */
export function labWorkRoot(entryId: string, runId: string, stage: string, env: NodeJS.ProcessEnv = process.env): string {
  return path.join(groveStateDir(env), "lab-work", `${entryId}-${runId}-${stage}`);
}

const CARD = /^\d{3,}\.json$/;

function hashOf(file: string): string | null {
  try {
    return crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
  } catch {
    return null;
  }
}

function copyFile(from: string, to: string): void {
  fs.mkdirSync(path.dirname(to), { recursive: true });
  const tmp = `${to}.${process.pid}.tmp`;
  fs.copyFileSync(from, tmp);
  fs.renameSync(tmp, to);
}

function listFiles(dir: string, rel = ""): string[] {
  let entries: fs.Dirent[];
  try {
    entries = fs.readdirSync(path.join(dir, rel), { withFileTypes: true });
  } catch {
    return [];
  }
  return entries.flatMap((e) => {
    const child = rel === "" ? e.name : `${rel}/${e.name}`;
    if (e.isDirectory()) return listFiles(dir, child);
    return e.name.endsWith(".tmp") ? [] : [child];
  });
}

/**
 * Keeps a working folder and a Lab entry in step. The files the agent works
 * on — drafts, scout notes, coverage, question cards, and the done marker —
 * are compared with the content they had when last in step: whichever side
 * changed since is copied to the other. An agent's change wins over the
 * Lab's when both changed, since the agent is the one writing these files.
 */
export class LabWorkSync {
  private readonly base = new Map<string, string>();

  constructor(
    private readonly lab: AgentPaths,
    readonly work: AgentPaths,
  ) {}

  /** The files kept in step, by path relative to the entry, on either side. */
  private tracked(): string[] {
    const files = new Set<string>(["scout.md", "coverage.json", "done"]);
    for (const root of [this.lab, this.work]) {
      for (const f of listFiles(root.artifactsDir)) files.add(`artifacts/${f}`);
      for (const f of listFiles(root.questionsDir)) if (CARD.test(f)) files.add(`questions/${f}`);
    }
    return [...files];
  }

  private resolve(root: AgentPaths, rel: string): string {
    return path.join(root.entryDir, ...rel.split("/"));
  }

  /** Brings both sides in step; returns the files copied, for testing. */
  sync(): string[] {
    const copied: string[] = [];
    for (const rel of this.tracked()) {
      const labFile = this.resolve(this.lab, rel);
      const workFile = this.resolve(this.work, rel);
      const lab = hashOf(labFile);
      const work = hashOf(workFile);
      const base = this.base.get(rel) ?? null;
      if (work !== null && work !== base) {
        if (work !== lab) copyFile(workFile, labFile);
        this.base.set(rel, work);
        copied.push(`work→lab ${rel}`);
      } else if (lab !== null && lab !== base) {
        if (lab !== work) copyFile(labFile, workFile);
        this.base.set(rel, lab);
        copied.push(`lab→work ${rel}`);
      }
    }
    return copied;
  }

  /** Starts a stage: the working folder holds what the Lab holds. */
  seed(): void {
    fs.mkdirSync(this.work.artifactsDir, { recursive: true });
    fs.mkdirSync(this.work.questionsDir, { recursive: true });
    this.sync();
  }

  /** Removes the done marker on both sides, so the agent finishes afresh. */
  clearDone(): void {
    for (const file of [this.lab.doneFile, this.work.doneFile]) fs.rmSync(file, { force: true, recursive: true });
    this.base.delete("done");
  }

  /** Ends a stage: what the agent last wrote reaches the Lab, then the folder goes. */
  dispose(): void {
    try {
      this.sync();
    } finally {
      fs.rmSync(this.work.entryDir, { recursive: true, force: true });
    }
  }
}
