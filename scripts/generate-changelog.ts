#!/usr/bin/env bun
/**
 * Generate changelog.json from conventional commits in git history.
 * Filters to user-facing commits (feat, fix, perf), groups by date.
 *
 * Usage: bun scripts/generate-changelog.ts [--limit N] [--output path]
 */

const USER_FACING_TYPES: Record<string, string> = {
  feat: "feature",
  fix: "fix",
  perf: "perf",
};

interface ChangelogEntry {
  type: string;
  message: string;
}

interface ChangelogGroup {
  date: string;
  entries: ChangelogEntry[];
}

// Parse CLI args
const args = process.argv.slice(2);
const limitIdx = args.indexOf("--limit");
const outputIdx = args.indexOf("--output");
const limit = limitIdx !== -1 ? parseInt(args[limitIdx + 1], 10) : 50;
const output =
  outputIdx !== -1
    ? args[outputIdx + 1]
    : "frontend/web/public/changelog.json";

// Get git log with format: hash|subject|date
const proc = Bun.spawnSync(["git", "log", "--format=%H|%s|%aI", "-200"]);
const gitLog = proc.stdout.toString();

const lines = gitLog.trim().split("\n").filter(Boolean);

const entries: { date: string; type: string; message: string }[] = [];

for (const line of lines) {
  const [, subject, dateStr] = line.split("|");
  if (!subject || !dateStr) continue;

  // Parse conventional commit: type(scope): message or type: message
  const match = subject.match(/^(\w+)(?:\([^)]*\))?:\s*(.+)$/);
  if (!match) continue;

  const [, commitType, rawMessage] = match;
  const mappedType = USER_FACING_TYPES[commitType];
  if (!mappedType) continue;

  // Capitalize first letter
  const message = rawMessage.charAt(0).toUpperCase() + rawMessage.slice(1);
  const date = dateStr.split("T")[0]; // YYYY-MM-DD

  entries.push({ date, type: mappedType, message });

  if (entries.length >= limit) break;
}

// Group by date
const grouped = new Map<string, ChangelogEntry[]>();
for (const entry of entries) {
  const group = grouped.get(entry.date) || [];
  group.push({ type: entry.type, message: entry.message });
  grouped.set(entry.date, group);
}

const changelog: ChangelogGroup[] = Array.from(grouped.entries()).map(
  ([date, entries]) => ({ date, entries })
);

await Bun.write(output, JSON.stringify(changelog, null, 2) + "\n");

console.log(
  `Generated ${output} with ${entries.length} entries in ${changelog.length} groups`
);
