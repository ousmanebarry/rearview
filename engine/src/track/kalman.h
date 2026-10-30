#pragma once

#include <Eigen/Dense>

#include "core/types.h"

namespace rearview::track {

// Constant-velocity Kalman filter over (cx, cy, aspect ratio w/h, height) and
// their velocities per frame, with noise scaled by the box height (the
// ByteTrack / DeepSORT formulation).
class KalmanBox {
public:
    using State = Eigen::Matrix<double, 8, 1>;
    using Cov = Eigen::Matrix<double, 8, 8>;
    using Meas = Eigen::Matrix<double, 4, 1>;

    explicit KalmanBox(const Box& b);
    void predict();
    void update(const Box& b);

    Box box() const;
    double height() const { return x_(3); }
    // Height change in pixels per frame (positive = growing = approaching).
    double height_velocity() const { return x_(7); }
    const State& state() const { return x_; }

    static Meas measure(const Box& b);

private:
    State x_;
    Cov p_;
};

}  // namespace rearview::track
