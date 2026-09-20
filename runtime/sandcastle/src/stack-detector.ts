import fs from "fs";
import path from "path";

export type TechStack = "GO" | "PYTHON" | "TYPESCRIPT";

export function detectStack(targetDir: string): TechStack {
  if (fs.existsSync(path.join(targetDir, "go.mod"))) {
    return "GO";
  }
  if (
    fs.existsSync(path.join(targetDir, "pyproject.toml")) ||
    fs.existsSync(path.join(targetDir, "requirements.txt"))
  ) {
    return "PYTHON";
  }
  return "TYPESCRIPT";
}
