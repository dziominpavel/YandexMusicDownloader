---
name: opsx-propose
description: Entry point for /opsx-propose. Propose a new OpenSpec change and generate all its artifacts (proposal, specs, design, tasks). Use when the user types /opsx-propose or says "opsx propose", "openspec propose".
---

This is the `/opsx-propose` entry point for this repository.

Load the `openspec-propose` skill with the `skill` tool and follow it exactly. Treat any text after the command — a kebab-case change name or a description of the wanted behavior — as that skill's input.

Do not restate or paraphrase the workflow here: `openspec-propose` is the single source of truth, and this alias exists only so the familiar `/opsx-*` names keep working in DSH.
