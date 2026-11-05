---
name: opsx-archive
description: Entry point for /opsx-archive. Archive a completed OpenSpec change and fold its deltas into the main specs. Use when the user types /opsx-archive or says "opsx archive", "openspec archive".
---

This is the `/opsx-archive` entry point for this repository.

Load the `openspec-archive-change` skill with the `skill` tool and follow it exactly. Treat any text after the command — a change name, or nothing when the active change is unambiguous — as that skill's input.

Do not restate or paraphrase the workflow here: `openspec-archive-change` is the single source of truth, and this alias exists only so the familiar `/opsx-*` names keep working in DSH.
