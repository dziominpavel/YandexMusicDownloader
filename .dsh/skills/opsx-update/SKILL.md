---
name: opsx-update
description: Entry point for /opsx-update. Revise an existing OpenSpec change's planning artifacts and keep them coherent. Use when the user types /opsx-update or says "opsx update", "openspec update change".
---

This is the `/opsx-update` entry point for this repository.

Load the `openspec-update-change` skill with the `skill` tool and follow it exactly. Treat any text after the command — a change name, a specific revision request, or nothing for a coherence review — as that skill's input.

Do not restate or paraphrase the workflow here: `openspec-update-change` is the single source of truth, and this alias exists only so the familiar `/opsx-*` names keep working in DSH.
