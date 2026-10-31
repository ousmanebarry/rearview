# Phase 9 (optional): Desktop viewer (about 2 weeks)

**Read first:** [../architecture.md](../architecture.md), [../data-model.md](../data-model.md), [../feature-specs.md](../feature-specs.md) (feature O)

Only start this after Phase 7 is done and Phase 10 polish is at least drafted.

- [x] **9.1 Qt app skeleton [agent]** — Qt 6 app in `viewer/` that opens the SQLite database read-only and lists trips.
- [x] **9.2 Video + timeline [agent, you review]** — play a trip's segments back to back (Qt Multimedia), switch front and rear, event markers on a timeline that jump the video when clicked.
- [x] **9.3 Map [agent]** — GPS track on a map (Qt Location with OpenStreetMap tiles), a marker that follows playback, event pins.
- [x] **9.4 Overlays [agent, you review]** — draw tracked boxes and TTC from `observations` on top of the video at the current time.
- [x] **9.5 Search bar [agent]** — run `rearview search --json` as a subprocess and show results; clicking one jumps to it.

Done when: the full demo (event list → click → video jumps → map follows → search finds a cyclist) runs without touching the terminal.
