#!/usr/bin/env node

import { readFileSync } from "node:fs";
import path from "node:path";

// === Grilling Workflow Artifacts ===

export interface GrillingArtifacts {
  context: string;           // Full transcript of specialist conversation
  spec: string;              // Implementation requirements, wireframes, specs
  outputPath?: string;       // Optional custom output path (empty uses default repo root)
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

  // Read context artifact from expected path per SANDCASTLE_JSON_CONTRACT
  let contextContent: string;
  try {
    contextContent = readFileSync(path.join(artifactsPath, "context.md"), "utf8") || "";
  } catch (err) {
    throw new Error(`Grilling workflow completed but context.md not found at expected path: ${String(err)}`);
  }

  // Read spec artifact from expected path per SANDCASTLE_JSON_CONTRACT
  let specContent: string;
  try {
    specContent = readFileSync(path.join(artifactsPath, "spec.md"), "utf8") || "";
  } catch (err) {
    throw new Error(`Grilling workflow completed but spec.md not found at expected path: ${String(err)}`);
  }

  return { context: contextContent, spec: specContent };
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

  const seen = new Set<string>();
  const patterns = [
    /feature[:\s](.+?)(?:[\.\n]|$)/gi,              // feature: description
    /implement[:\s](.+?)(?:[\.\n]|$)/gi,            // implement: description
    /requirement[:\s](.+?)(?:[\.\n]|$)/gi,         // requirement: description
    /task[:\s](.+?)(?:[\.\n]|$)/gi,                 // task: description
    /^-[\s]+(.+)$/gm,                                // bullet points (not in quotes)
    /\*\*(.+?)\*\*$/,                                  // bold text at end of line
  ];

  for (const entry of transcript) {
    const lines = entry.message.split("\n");

    for (const line of lines) {
      let found = false;

      // Try each pattern in order
      for (const pattern of patterns) {
        const match = line.match(pattern);
        if (match && !found) {
          const content = match[1]?.trim();
          if (content && content.length > 5 && !seen.has(content)) {
            seen.add(content);
            spec += `• ${content}\n\n`;
            found = true;
            break;
          }
        }
      }
    }
  }

  // Add default placeholder if no explicit specs found
  const trimmedSpec = spec.replace(/[\s]*\n\s*$/, "");
  if (trimmedSpec === "### Specification") {
    spec = `### Specification\n\n**Summary of Key Points**\n\n`;

    // Extract any key terms or concepts mentioned in the transcript
    for (const entry of transcript) {
      const messageLower = entry.message.toLowerCase();
      if (messageLower.includes("implement") ||
          messageLower.includes("build") ||
          messageLower.includes("create") ||
          messageLower.includes("add")) {
        const sentences = entry.message.split(/[.!?]+/);
        for (const sent of sentences) {
          const trimmed = sent.trim();
          if (trimmed.length > 20 && !trimmed.startsWith(">") && !seen.has(trimmed)) {
            seen.add(trimmed);
            spec += `${trimmed}.\n\n`;
          }
        }
      }
    }

    // Add generic action items if still empty
    if (spec.trim().endsWith("### Specification\n")) {
      spec = "### Specification\n\n**Implementation Guidelines**\n\n" +
            "- Review the context document for complete conversation transcript\n" +
            "- Identify all requirements and feature specifications mentioned\n" +
            "- Create implementation plan based on specialist recommendations\n";
    }
  }

  return spec.trim() + "\n";
}
