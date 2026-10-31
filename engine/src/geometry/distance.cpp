#include "geometry/distance.h"

#include <opencv2/imgproc.hpp>
#include <sstream>

namespace rearview::geometry {

std::map<int, double> default_class_heights() {
    // YOLOX's "truck" is mostly pickups and vans on these roads, so 2.0 m, not a semi's 4 m.
    return {{0, 1.7}, {1, 1.6}, {2, 1.5}, {3, 1.4}, {5, 3.2}, {7, 2.0}};
}

std::optional<std::map<int, double>> parse_class_heights(const std::string& s) {
    auto h = default_class_heights();
    std::istringstream in(s);
    std::string item;
    while (std::getline(in, item, ',')) {
        const auto eq = item.find('=');
        if (eq == std::string::npos) return std::nullopt;
        const std::string name = item.substr(0, eq);
        int id = -1;
        for (const auto& [cid, cname] : kKeptClasses) {
            if (cname == name) id = cid;
        }
        if (id < 0) return std::nullopt;
        try {
            h[id] = std::stod(item.substr(eq + 1));
        } catch (const std::exception&) {
            return std::nullopt;
        }
    }
    return h;
}

std::optional<double> distance_m(const Box& b, int cls, const Calibration& cal,
                                 const std::map<int, double>& heights) {
    const auto it = heights.find(cls);
    if (it == heights.end() || b.height() < 2) return std::nullopt;
    const auto n = cal.normalize({{b.cx(), b.y1}, {b.cx(), b.y2}});
    const double dy = n[1].y - n[0].y;
    if (!(dy > 1e-6)) return std::nullopt;
    return it->second / dy;
}

std::optional<double> ttc_s(double h, double dh_dt, double min_h) {
    if (h < min_h || !(dh_dt > 1e-6)) return std::nullopt;
    return h / dh_dt;
}

std::vector<cv::Point2f> PathRegion::pixels(int w, int h) const {
    std::vector<cv::Point2f> out;
    for (const auto& c : corners)
        out.emplace_back(c.x * static_cast<float>(w), c.y * static_cast<float>(h));
    return out;
}

bool PathRegion::contains(const Box& b, int w, int h) const {
    return cv::pointPolygonTest(pixels(w, h), cv::Point2f(b.cx(), b.y2), false) >= 0;
}

std::optional<PathRegion> parse_path(const std::string& s) {
    PathRegion r;
    std::istringstream in(s);
    std::string pair;
    int i = 0;
    while (std::getline(in, pair, ';')) {
        if (i >= 4) return std::nullopt;
        float x = 0, y = 0;
        char comma = 0;
        std::istringstream p(pair);
        if (!(p >> x >> comma >> y) || comma != ',') return std::nullopt;
        r.corners[static_cast<size_t>(i++)] = {x, y};
    }
    if (i != 4) return std::nullopt;
    return r;
}

}  // namespace rearview::geometry
