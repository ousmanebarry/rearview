#pragma once

#include <map>
#include <vector>

#include "core/types.h"
#include "track/kalman.h"

namespace rearview::track {

struct TrackerConfig {
    float high_conf = 0.5f;       // detections at or above this start and keep tracks
    float match_iou_high = 0.2f;  // first pass: high detections vs tracked and lost tracks
    float match_iou_low = 0.5f;   // second pass: low detections vs leftover tracked tracks
    float match_iou_new = 0.3f;   // tentative tracks vs leftover high detections
    int confirm_hits = 3;         // tentative -> confirmed
    int max_lost = 30;            // frames a lost track is kept before removal
};

enum class TrackState { Tentative, Confirmed, Lost };

struct Track {
    int id = 0;
    TrackState state = TrackState::Tentative;
    KalmanBox kf;
    int hits = 1;
    int lost_frames = 0;
    float conf = 0;
    std::map<int, int> class_votes;

    int cls() const;
};

// What the tracker reports for one confirmed track in the current frame.
struct TrackedObject {
    int id = 0;
    int cls = 0;
    float conf = 0;
    Box box;                // smoothed
    double height = 0;      // smoothed box height, px
    double height_vel = 0;  // px per frame, positive when the box grows
};

// ByteTrack-style multi-object tracker: Kalman prediction, then IoU matching of
// high-confidence detections first and low-confidence ones to the leftovers.
class Tracker {
public:
    explicit Tracker(TrackerConfig cfg = {}) : cfg_(cfg) {}

    // Advances one frame. Returns the confirmed tracks matched in this frame.
    std::vector<TrackedObject> update(const std::vector<Detection>& dets);
    void reset();
    const std::vector<Track>& tracks() const { return tracks_; }

private:
    TrackerConfig cfg_;
    std::vector<Track> tracks_;
    int next_id_ = 1;
};

// Greedy one-to-one matching of rows to columns by highest IoU above min_iou.
// Returns (row, col) pairs.
std::vector<std::pair<int, int>> match_iou(const std::vector<Box>& rows,
                                           const std::vector<Box>& cols, float min_iou);

}  // namespace rearview::track
