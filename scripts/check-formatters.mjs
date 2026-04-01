import { access } from "node:fs/promises";
import { constants } from "node:fs";
import { spawnSync } from "node:child_process";

async function ensureExecutable(path) {
  try {
    await access(path, constants.X_OK);
  } catch (error) {
    throw new Error(`Missing executable: ${path}`);
  }
}

await ensureExecutable("./scripts/format-files.sh");

const result = spawnSync("./scripts/format-files.sh", ["--all"], {
  stdio: "inherit",
  shell: false,
});

if (result.status !== 0) {
  process.exit(result.status ?? 1);
}
