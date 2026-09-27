#!/usr/bin/env node

import { readFileSync } from "node:fs";
import path from "node:path";

// === Grilling Workflow Artifacts ===

export interface GrillingArtifacts {
  context: string;           // Full transcript of specialist conversation
  spec: string;              // Implementation requirements, wireframes, specs
}

/**
 * Generate grilling artifacts (context document and spec) from workflow transcript.
 * This function reads the specialist conversation and extracts:
 * - Context document: full transcript with timestamps and agent names
 * - Spec: actionable items, requirements, and implementation details
 */
export async function generateGrillingArtifacts(
  runId: string,
  commonDir: string
): Promise<GrillingArtifacts> {
  const artifactsPath = path.join(commonDir, "agent-flow", "grilling");

  try {
    // Read context artifact from expected path per SANDCASTLE_JSON_CONTRACT
    let contextContent: string;
    try {
      contextContent = readFileSync(path.join(artifactsPath, "context.md"), "utf8") || "";
    } catch {
      throw new Error("Grilling workflow completed but context.md not found at expected path");
    }

    // Read spec artifact from expected path per SANDCASTLE_JSON_CONTRACT
    let specContent: string;
    try {
      specContent = readFileSync(path.join(artifactsPath, "spec.md"), "utf8") || "";
    } catch {
      throw new Error("Grilling workflow completed but spec.md not found at expected path");
    }

    return { context: contextContent, spec: specContent };
  } catch (err) {
    throw new Error(`Failed to generate grilling artifacts: ${String(err)}`);
  }
}

export async function readGrillingArtifacts(
  commonDir: string
): Promise<GrillingArtifacts> {
  const contextPath = path.join(commonDir, "agent-flow", "grilling", "context.md");
  const specPath = path.join(commonDir, "agent-flow", "grilling", "spec.md");

  let content: string;

  try {
    content = readFileSync(contextPath, "utf8");
    return { context: content, spec: "" };
  } catch {
    // Context may not exist yet
  }

  try {
    content = readFileSync(specPath, "utf8");
    return { context: "", spec: content };
  } catch {
    // Neither exists - workflow hasn't completed with artifacts
    throw new Error("Grilling workflow completed but neither context nor spec artifacts found");
  }
}

/**
 * Format a full transcript document from specialist conversation entries.
 */
function formatContextDocument(transcript: Array<{ agent: string; message: string; timestamp: string }>): string {
  let doc = "### Grilling Session Transcript\n\n";
  for (const entry of transcript) {
    const agentName = entry.agent;
    const timestamp = entry.timestamp;
    const message = entry.message;
    doc += `[**${agentName}** @ ${timestamp}]\n> ${message}\n\n`;
  }
  return doc.trim() + "\n";
}

/**
 * Extract actionable requirements and implementation details from conversation.
 */
function formatSpec(transcript: Array<{ agent: string; message: string }>): string {
  let spec = "### Specification\n\n";

  // Extract key items mentioned in the conversation
  for (const entry of transcript) {
    const lines = entry.message.split("\n");
    for (const line of lines) {
      if (line.toLowerCase().includes("feature:") ||
          line.toLowerCase().includes("implement:") ||
          line.includes("-") && !line.startsWith(">")) {
        spec += line.trim() + "\n\n";
      }
    }
  }

  // Add default placeholder if no explicit specs found
  if (spec.trim() === "### Specification\n") {
    spec = "### Specification\n\n" +
          "**Action Items**\n\n" +
          "- Review context document for full conversation details\n" +
          "- Extract key requirements from specialist responses\n" +
          "- Implement approved features based on specification\n";
  }

  return spec.trim() + "\n";
}
