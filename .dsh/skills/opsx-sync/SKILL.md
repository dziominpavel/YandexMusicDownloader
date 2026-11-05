---
name: opsx-sync
description: Entry point for /opsx-sync. Sync delta specs from an OpenSpec change into the main specs without archiving the change. Use when the user types /opsx-sync or says "opsx sync", "openspec sync".
---

This is the `/opsx-sync` entry point for this repository.

Load the `openspec-sync-specs` skill with the `skill` tool and follow it exactly. Treat any text after the command — a change name, or nothing when the active change is unambiguous — as that skill's input.

Do not restate or paraphrase the workflow here: `openspec-sync-specs` is the single source of truth, and this alias exists only so the familiar `/opsx-*` names keep working in DSH.
