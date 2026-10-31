# Phase 7: Natural-language search (weeks 9–10)

**Read first:** [../../../protocol/README.md](../../../protocol/README.md), [data-model.md](../../data-model.md) (added later: `keyframes`), [design: features](../../design.md#features) (feature I), [design: testing](../../design.md#testing-and-evaluation)

**Walkthrough:** [phase-7-search.md](../../walkthroughs/phase-7-search.md)

Goal: `rearview search "red truck"` returns the right moments from your own drives, with a measured hit rate.

- [x] **7.1 CLIP image encoder [agent, you review]**
  - `src/embed`: CLIP ViT-B/32 image model via ONNX Runtime. Preprocess: resize the short side to 224, centre-crop 224×224, RGB, scale to 0–1, normalize with CLIP's mean (0.48145466, 0.4578275, 0.40821073) and std (0.26862954, 0.26130258, 0.27577711). L2-normalize the 512-value output.
  - Dashcam frames are very wide, so a centre crop loses the sides. Also try three crops (left, centre, right) per keyframe and keep whichever scores better in 7.6.
  - Done when: embedding the same frame twice gives identical vectors, and a sanity test ranks a photo of a car above a photo of a tree for the text "a car".
- [x] **7.2 BPE tokenizer + text encoder [agent, you review]**
  - Implement CLIP's byte-pair-encoding tokenizer in C++ from its vocabulary and merges files: lowercase, clean whitespace, split into words, apply merges, add start token 49406 and end token 49407, pad to 77 tokens.
  - Run the CLIP text model, L2-normalize the output.
  - Done when: golden tests match reference token IDs for 20 sample strings (taken once from a trusted implementation and committed).
- [x] **7.3 Protocol commands [agent]**
  - Add `embed_frames` and `embed_text` to both sides and the spec, with round-trip tests. Adding commands is not a breaking change, so the protocol version stays the same.
- [x] **7.4 Keyframe indexing [agent, you review]**
  - Migration `0002_search.sql` adds `keyframes`. Planner creates `embed_frames` jobs at one keyframe per second (front camera first). Store vectors as 512 little-endian float32 values.
  - Done when: `rearview process --embed` fills `keyframes` for a day of footage, and re-running skips finished segments.
- [x] **7.5 Search [agent, you review]**
  - `internal/search`: load all vectors into memory (a week of footage is well under 100 MB), get the query vector from `embed_text` with a prompt template such as "a dashcam photo of {query}", score by dot product, keep the top 50.
  - Group hits within 5 seconds of each other in the same segment into one moment; rank moments by their best score.
  - `rearview search "<query>" [--limit 10] [--clip]` prints time, trip, score and location; `--clip` cuts a clip for each result.
  - Done when: a query over a week of footage answers in under 1 second.
- [x] **7.6 Evaluate [agent, you review]**
  - Draft 20–30 candidate test queries ("snowy road", "bus", "cyclist", "night rain", "traffic light", "bridge") by sampling keyframes, each with the time range where it appears, in `testdata/search_queries.csv`.
  - `rearview eval --search` reports the share of queries with a correct moment in the top 5. Compare centre-crop vs three-crop, and prompt template vs plain query.
  - Input needed: confirm or correct each query's expected time range, since only someone who knows the footage can vouch for them.
  - Done when: the number is recorded in `docs/plan/decisions.md`.

**Phase 7 deliverable:** search that works on your own drives, with a hit rate. With this, the core project is complete.
