---
version: 1
slug: "web"
primary_target: "web"
related_targets: []
---

Scope: whole Axolot web app. Mode: Operate, the Direction page reads as a summary for a non-technical reader.
Audience and job: see PRODUCT.md. Constraints: docs/poc.md section UI, css-conventions, no new runtime dependency.
Unresolved: none.

## Direction contract

THESIS: A working comparison lab whose verdict is readable in one glance and provable in two clicks. Refuses the generic dashboard of KPI tiles and purple gradient.

OWN-WORLD: Deep pond water ground (blue-green ink), pale ink, axolotl gill pink as the single brand accent. Glass only on the navigation and floating panels. Flat, hairline tables. System sans, monospace only for model tags and outputs. Icons draw themselves on hover.

STORY: The visitor understands which model suits which task on this machine, believes it because scoring is deterministic and sources open, then watches a case run live.

FIRST VIEWPORT: Floating glass bar at top with logo and five destinations. Left aligned title "Quel modèle pour quelle tâche ?", machine and campaign line beside the campaign picker. Full-width verdict table: task rows, Qualité, Vitesse, Compromis columns. Four-step "how it works" strip below, ending with the live-run action.

FORM: pinned by docs/poc.md, no concept roll. Signature interaction: live neural network on canvas driven by GET /api/events, one impulse per token, ripple with score at item end. seed: none (unavailable)

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
