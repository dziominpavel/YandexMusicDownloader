---
name: opsx-apply
description: Entry point for /opsx-apply. Implement the tasks of an existing OpenSpec change. Use when the user types /opsx-apply or says "opsx apply", "openspec apply", "openspec implement".
---

This is the `/opsx-apply` entry point for this repository.

Load the `openspec-apply-change` skill with the `skill` tool and follow it exactly. Treat any text after the command — a change name, or nothing at all when the active change is unambiguous — as that skill's input.

Do not restate or paraphrase the workflow here: `openspec-apply-change` is the single source of truth, and this alias exists only so the familiar `/opsx-*` names keep working in DSH.
