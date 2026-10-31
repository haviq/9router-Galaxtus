### Jailbreak: system injection mode per preset, and a richer Model Status

Prompt injection was always a leading system message. A preset now carries its
own **inject mode**, so an operator can choose how the prompt reaches the
outbound body:

- `prepend` (default) — ahead of the existing system content, original intact
  after it. This is the pre-feature behaviour, so an unset mode changes nothing.
- `append` — after the existing system content, for a preset that refines an
  operator instruction rather than leading it.
- `replace` — the existing system content is dropped and only the prompt is
  sent, for a hard system override.

The mode is wire-agnostic: it applies to both the OpenAI `messages` shape and
the Claude top-level `system` (string or block array), and a body with no system
content at all still receives the prompt as a leading system message. It is
stored on `JailbreakDoc.injectMode`, validated at save time by
`NormalizeJailbreakInjectMode` (an unknown mode is a 400, never a stored typo),
and applied on the request path by `applySystemPrompt` in place of the old
`prependSystemPrompt`. The preview endpoint renders the same result over the
caller's own messages and reports the resolved `injectMode`.

**Model Status** (`POST /api/jailbreak/models/status`, the Model Status page)
now carries the winning preset's metadata alongside the verdict: `presetKind`
(the instruction-file convention), `injectMode`, and `usage` (the preset's
lifetime injection count). The preset list and status table surface them, so an
operator can tell what would be applied, and whether a preset earns its keep,
without opening it. Both are read once per sweep rather than per row.

**Persona** (the jailbreak injection surface) is now **Prompt Injection** in the
sidebar and page header, and visible out of the box. It was labelled "Persona"
and in the default-hidden list, so the feature read as missing: the sidebar drew
nothing and an operator had to find Settings → Feature Visibility to reveal it.
Only **Plans** stays hidden by default now.

**Model Status** is a live check, not just a verdict list. Each row carries a
measured **latency** from a real probe through the provider —
`POST /api/models/test` with the model id, the same probe the per-model Test
button runs, so the server's own clock is the number shown, not the browser's.
A row has a one-click **bolt** to ping it, and the header has **Ping N** to
sweep the current filter with bounded concurrency (4 workers), a **sort** control
(default / fastest first / down first) and a **summary** over the rows that
answered (up, down, median, min, max). `probeLimit` caps a sweep at 10/20/50/100
rows so a large catalog never fires every probe at once.

**Model Status heatmap.** The page reads two ways, chosen by a Heatmap/List
toggle. The **heatmap** draws every model as a square grouped by provider — one
row per provider, one square per model, the GitHub-contribution shape — so the
whole catalog is visible at a glance instead of paged. A square is green when a
probe just succeeded, red when it failed, terracotta when the model is injecting,
amber when a deny list excludes it, and grey when off or untested; clicking one
pings that model. The groups are built from the same filter and search the list
uses, so one control set drives both readings. Square size follows the group, so
a provider with hundreds of models stays on one row.

**SYADAGENTIC pack.** The Prompt Injection import dialog gains **Install
SYADAGENTIC pack**, which lands the [`Sekolah76/syadagentic`](
https://github.com/Sekolah76/syadagentic) prompt-injection collection as presets.
That collection is one file (`prompt-injection/templates.md`) holding 40 numbered
techniques (`## T1. …`–`## T119. …`), not one file per model, so the generic
importer would store it as a single preset. The pack endpoint
(`POST /api/jailbreak/import/syadagentic`) parses the `T<N>. TITLE` sections
instead: one preset per technique, named from its number and title, labelled
`SYADAGENTIC`, prompt verbatim, no keywords (a technique is not bound to a model
by name). Re-running refreshes the pack in place rather than duplicating it. The
fetch goes through the same guarded importer — GitHub-only, size-bounded — as
every other import.

Verified with `go test ./internal/db/ ./internal/handlers/chat/
./internal/handlers/dashboard/ ./web/...` (new table-driven cases for the three
inject modes, the status enrichment, and the SYADAGENTIC parser/pack import),
`bun run build`, `bun run ratchet:svelte` (0 unresolved identifiers, count held at
the 77 baseline), and `bun test src scripts` (659 pass).
