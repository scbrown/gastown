# Agent Skills Convention

Skills are authored in `.agents/skills/<name>/SKILL.md` (the source of truth).
A file symlink in `docs/skills/` makes the skill browsable alongside design
docs, and a folder symlink in `.claude/skills/` exposes it to Claude Code.

```
.agents/skills/<name>/SKILL.md          <- source of truth
docs/skills/<name>/SKILL.md             <- file symlink -> .agents/skills/<name>/SKILL.md
.claude/skills/<name>/                  <- folder symlink -> .agents/skills/<name>/
```

## Why this layout

- **`.agents/skills/`** is the agent-agnostic directory (used by the
  [Agent Skills](https://agentskills.io) open standard / `npx skills`).
  Real directories here so `npx skills list` discovers them.
- **`docs/skills/`** file symlink keeps the skill browsable next to its
  design docs, test plans, and specs without duplicating content.
- **`.claude/skills/`** folder symlink is the Claude Code discovery directory.

## Adding a new skill

```bash
# 1. Create the skill in .agents/skills/
mkdir -p .agents/skills/my-feature
# write .agents/skills/my-feature/SKILL.md

# 2. File-symlink into docs/skills/ for documentation browsing
mkdir -p docs/skills/my-feature
ln -s ../../../.agents/skills/my-feature/SKILL.md docs/skills/my-feature/SKILL.md

# 3. Folder-symlink into .claude/skills/ for Claude Code discovery
ln -s ../../.agents/skills/my-feature .claude/skills/my-feature
```
