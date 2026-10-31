#pragma once

#include <array>
#include <map>
#include <opencv2/core.hpp>
#include <optional>
#include <string>
#include <vector>

#include "core/types.h"
#include "geometry/calibration.h"

namespace rearview::geometry {

// Real heights in metres per class, for distance from box height.
std::map<int, double> default_class_heights();
// Parses "car=1.5,truck=2.0" over the defaults.
std::optional<std::map<int, double>> parse_class_heights(const std::string& s);

// Distance in metres from a box: the top-centre and bottom-centre points are
// undistorted to normalised coordinates, and d = H / (y_bottom - y_top), which
// is f * H / h for an ideal pinhole camera. nullopt for classes without a height.
std::optional<double> distance_m(const Box& b, int cls, const Calibration& cal,
                                 const std::map<int, double>& heights);

// Time to collision from the box height h (px) and its growth rate (px/s):
// h / (dh/dt). nullopt when the box is not growing or is shorter than min_h.
std::optional<double> ttc_s(double h, double dh_dt, double min_h);

// The in-path region: a quadrilateral given as fractions of the frame
// (bottom-left, top-left, top-right, bottom-right).
struct PathRegion {
    std::array<cv::Point2f, 4> corners{
        {{0.30f, 0.95f}, {0.46f, 0.55f}, {0.54f, 0.55f}, {0.70f, 0.95f}}};

    std::vector<cv::Point2f> pixels(int w, int h) const;
    // True when the box's bottom-centre lies inside the region.
    bool contains(const Box& b, int w, int h) const;
};

// Parses "x,y;x,y;x,y;x,y" (fractions).
std::optional<PathRegion> parse_path(const std::string& s);

}  // namespace rearview::geometry
