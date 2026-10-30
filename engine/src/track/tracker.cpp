#include "track/tracker.h"

#include <algorithm>
#include <tuple>

#include "detect/nms.h"

namespace rearview::track {

int Track::cls() const {
    int best = 0, votes = -1;
    for (const auto& [c, n] : class_votes) {
        if (n > votes) best = c, votes = n;
    }
    return best;
}

std::vector<std::pair<int, int>> match_iou(const std::vector<Box>& rows,
                                           const std::vector<Box>& cols, float min_iou) {
    std::vector<std::tuple<float, int, int>> cands;
    for (int r = 0; r < static_cast<int>(rows.size()); ++r) {
        for (int c = 0; c < static_cast<int>(cols.size()); ++c) {
            const float v = detect::iou(rows[static_cast<size_t>(r)], cols[static_cast<size_t>(c)]);
            if (v > min_iou) cands.emplace_back(v, r, c);
        }
    }
    std::stable_sort(cands.begin(), cands.end(),
                     [](const auto& a, const auto& b) { return std::get<0>(a) > std::get<0>(b); });
    std::vector<bool> used_r(rows.size()), used_c(cols.size());
    std::vector<std::pair<int, int>> out;
    for (const auto& [v, r, c] : cands) {
        if (used_r[static_cast<size_t>(r)] || used_c[static_cast<size_t>(c)]) continue;
        used_r[static_cast<size_t>(r)] = used_c[static_cast<size_t>(c)] = true;
        out.emplace_back(r, c);
    }
    return out;
}

void Tracker::reset() { tracks_.clear(); }

std::vector<TrackedObject> Tracker::update(const std::vector<Detection>& dets) {
    for (auto& t : tracks_) t.kf.predict();

    std::vector<int> high, low;
    for (int i = 0; i < static_cast<int>(dets.size()); ++i) {
        (dets[static_cast<size_t>(i)].conf >= cfg_.high_conf ? high : low).push_back(i);
    }
    std::vector<bool> matched(tracks_.size(), false);
    std::vector<bool> det_used(dets.size(), false);

    auto assign = [&](Track& t, const Detection& d) {
        t.kf.update(d.box);
        t.conf = d.conf;
        t.class_votes[d.cls]++;
        t.lost_frames = 0;
        ++t.hits;
        if (t.state == TrackState::Lost) t.state = TrackState::Confirmed;
        if (t.state == TrackState::Tentative && t.hits >= cfg_.confirm_hits) {
            t.state = TrackState::Confirmed;
        }
    };

    // Runs one matching pass between the tracks passing `want` and detections `cand`.
    auto pass = [&](auto want, const std::vector<int>& cand, float min_iou) {
        std::vector<int> ti;
        std::vector<Box> tb, db;
        for (int i = 0; i < static_cast<int>(tracks_.size()); ++i) {
            if (!matched[static_cast<size_t>(i)] && want(tracks_[static_cast<size_t>(i)])) {
                ti.push_back(i);
                tb.push_back(tracks_[static_cast<size_t>(i)].kf.box());
            }
        }
        std::vector<int> di;
        for (int d : cand) {
            if (!det_used[static_cast<size_t>(d)]) {
                di.push_back(d);
                db.push_back(dets[static_cast<size_t>(d)].box);
            }
        }
        for (const auto& [r, c] : match_iou(tb, db, min_iou)) {
            const int t = ti[static_cast<size_t>(r)];
            const int d = di[static_cast<size_t>(c)];
            matched[static_cast<size_t>(t)] = true;
            det_used[static_cast<size_t>(d)] = true;
            assign(tracks_[static_cast<size_t>(t)], dets[static_cast<size_t>(d)]);
        }
    };

    auto established = [](const Track& t) { return t.state != TrackState::Tentative; };
    auto tracked = [](const Track& t) { return t.state == TrackState::Confirmed; };
    auto tentative = [](const Track& t) { return t.state == TrackState::Tentative; };
    pass(established, high, cfg_.match_iou_high);
    pass(tracked, low, cfg_.match_iou_low);
    pass(tentative, high, cfg_.match_iou_new);

    std::vector<Track> kept;
    std::vector<TrackedObject> out;
    for (size_t i = 0; i < tracks_.size(); ++i) {
        Track& t = tracks_[i];
        if (!matched[i]) {
            if (t.state == TrackState::Tentative) continue;
            t.state = TrackState::Lost;
            if (++t.lost_frames > cfg_.max_lost) continue;
        } else if (t.state == TrackState::Confirmed) {
            out.push_back({t.id, t.cls(), t.conf, t.kf.box(), t.kf.height(), t.kf.height_velocity()});
        }
        kept.push_back(std::move(t));
    }
    tracks_ = std::move(kept);

    for (int d : high) {
        if (det_used[static_cast<size_t>(d)]) continue;
        const auto& det = dets[static_cast<size_t>(d)];
        Track t{next_id_++, TrackState::Tentative, KalmanBox(det.box), 1, 0, det.conf, {{det.cls, 1}}};
        if (cfg_.confirm_hits <= 1) {
            t.state = TrackState::Confirmed;
            out.push_back({t.id, det.cls, det.conf, det.box, t.kf.height(), 0});
        }
        tracks_.push_back(std::move(t));
    }
    return out;
}

}  // namespace rearview::track
