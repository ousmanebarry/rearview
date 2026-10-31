# Build plan

The plan Rearview was built from, split into phases whose checkboxes track progress. Agents read [.github/AGENTS.md](../../.github/AGENTS.md) first, then this file. The design it implements is in [../design.md](../design.md); decisions, measured results and gotchas are in [decisions.md](decisions.md).

## Phases

| Phase        | Tasks                                                              | Walkthrough                                                                 | Status                                                                 |
| ------------ | ------------------------------------------------------------------ | --------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| 0            | [phases/phase-0-setup.md](phases/phase-0-setup.md)                 | [go-primer.md](../walkthroughs/go-primer.md)                                | Done except 0.5 (CI green on a PR) and 0.7 (20 labels; 15 so far)      |
| 1            | [phases/phase-1-go-foundation.md](phases/phase-1-go-foundation.md) | [phase-1-import-gps-trips.md](../walkthroughs/phase-1-import-gps-trips.md)  | Done                                                                   |
| 2            | [phases/phase-2-motion-events.md](phases/phase-2-motion-events.md) | [phase-2-motion-events.md](../walkthroughs/phase-2-motion-events.md)        | Done                                                                   |
| 3            | [phases/phase-3-engine.md](phases/phase-3-engine.md)               | [phase-3-engine.md](../walkthroughs/phase-3-engine.md)                      | Done                                                                   |
| 4            | [phases/phase-4-connect.md](phases/phase-4-connect.md)             | [phase-4-pipeline.md](../walkthroughs/phase-4-pipeline.md)                  | Done                                                                   |
| 5            | [phases/phase-5-near-miss.md](phases/phase-5-near-miss.md)         | [phase-5-near-miss.md](../walkthroughs/phase-5-near-miss.md)                | 5.3, 5.4 done; 5.1, 5.2, 5.5 need calibration recordings and labels    |
| 6            | [phases/phase-6-performance.md](phases/phase-6-performance.md)     | [phase-6-performance.md](../walkthroughs/phase-6-performance.md)            | Mostly done: 6.3 has DirectML only (MIGraphX not built); 6.4 and 6.7 built and measured, no gain |
| 7            | [phases/phase-7-search.md](phases/phase-7-search.md)               | [phase-7-search.md](../walkthroughs/phase-7-search.md)                      | Done (21% top-5, below the 70% target)                                 |
| 8 (optional) | [phases/phase-8-extras.md](phases/phase-8-extras.md)               | [phase-8-blur-score-report.md](../walkthroughs/phase-8-blur-score-report.md) | 8.2, 8.3 done; 8.1 needs a frame-by-frame check of blurred clips       |
| 9 (optional) | [phases/phase-9-viewer.md](phases/phase-9-viewer.md)               | [phase-9-viewer.md](../walkthroughs/phase-9-viewer.md)                      | Done                                                                   |
| 10           | [phases/phase-10-polish.md](phases/phase-10-polish.md)             | [../writeup.md](../writeup.md), [../interview-prep.md](../interview-prep.md) | Done except 10.2 (the demo recording; script in [../demo-script.md](../demo-script.md)) |

## Definition of done for the whole project

- `rearview import` → `rearview process` → `rearview events` works on a full week of real drives.
- Near-miss detection has a measured precision and recall against labelled events.
- `rearview search` finds the right moment in the top 5 for most test queries.
- `rearview bench` reports minutes of processing per hour of footage, with a history of improvements.
- The README has the architecture, protocol, results tables and a 60–90 second demo video.

## Working with coding agents

Each checkbox in the phase files is one agent task (roughly one PR) with its own "Done when" line.

- Each phase file starts with a "Read first" list naming the spec sections its tasks need. Agents read those and nothing else from the plan.
- The agent ticks the task's box in the same PR that completes it, and records any decision or measured result in [decisions.md](decisions.md).
- **[agent]**: light review. **[agent, you review]**: close review, plus a walkthrough of the core logic in the phase's file in `docs/walkthroughs/`. **Input needed**: something an agent can't produce (footage, a real-world measurement); the agent stops and asks.
- Running several agents in parallel: turn each task into a GitHub issue and have each PR close its issue, so two agents never pick the same task.

**Task prompt template**

```markdown
Task: <number and name, e.g. 5.2 Distance estimation>
Read: .github/AGENTS.md, docs/plan/README.md, docs/plan/phases/<phase file>,
and the sections in that phase's "Read first" list.
Goal: do exactly this task. Its "Done when" line is the acceptance test.
Constraints: only touch <directories>. Tests first. Run the full suite.
Input provided: <footage paths, measurements, labels, if the task needs them>
When done: tick the task's box, record decisions and results in
docs/plan/decisions.md, and update the phase walkthrough in docs/walkthroughs/.
```

**Review checklist for every agent PR**

- [ ] Tests exist, ran, and would fail if the feature broke.
- [ ] No goroutine can leak (each one exits on context cancel).
- [ ] Protocol or schema changes are in the spec / a new migration.
- [ ] No footage, models or secrets committed.
- [ ] You can explain the change in two sentences.
