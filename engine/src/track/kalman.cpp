#include "track/kalman.h"

#include <algorithm>

namespace rearview::track {

namespace {
constexpr double kStdPos = 1.0 / 20;
constexpr double kStdVel = 1.0 / 160;
}  // namespace

KalmanBox::Meas KalmanBox::measure(const Box& b) {
    const double h = std::max(1.0f, b.height());
    return {b.cx(), b.cy(), b.width() / h, h};
}

KalmanBox::KalmanBox(const Box& b) {
    const Meas m = measure(b);
    x_.setZero();
    x_.head<4>() = m;
    const double h = m(3);
    Eigen::Matrix<double, 8, 1> std;
    std << 2 * kStdPos * h, 2 * kStdPos * h, 1e-2, 2 * kStdPos * h, 10 * kStdVel * h,
        10 * kStdVel * h, 1e-5, 10 * kStdVel * h;
    p_ = std.array().square().matrix().asDiagonal();
}

void KalmanBox::predict() {
    Eigen::Matrix<double, 8, 8> f = Eigen::Matrix<double, 8, 8>::Identity();
    for (int i = 0; i < 4; ++i) f(i, i + 4) = 1;
    const double h = x_(3);
    Eigen::Matrix<double, 8, 1> std;
    std << kStdPos * h, kStdPos * h, 1e-2, kStdPos * h, kStdVel * h, kStdVel * h, 1e-5, kStdVel * h;
    const Cov q = std.array().square().matrix().asDiagonal();
    x_ = f * x_;
    p_ = f * p_ * f.transpose() + q;
}

void KalmanBox::update(const Box& b) {
    const Meas z = measure(b);
    Eigen::Matrix<double, 4, 8> hm = Eigen::Matrix<double, 4, 8>::Zero();
    for (int i = 0; i < 4; ++i) hm(i, i) = 1;
    const double h = x_(3);
    Eigen::Matrix<double, 4, 1> std;
    std << kStdPos * h, kStdPos * h, 1e-1, kStdPos * h;
    const Eigen::Matrix4d r = std.array().square().matrix().asDiagonal();
    const Eigen::Matrix4d s = hm * p_ * hm.transpose() + r;
    const Eigen::Matrix<double, 8, 4> k = p_ * hm.transpose() * s.inverse();
    x_ = x_ + k * (z - hm * x_);
    p_ = (Cov::Identity() - k * hm) * p_;
}

Box KalmanBox::box() const {
    const double h = std::max(1.0, x_(3));
    const double w = x_(2) * h;
    return {static_cast<float>(x_(0) - w / 2), static_cast<float>(x_(1) - h / 2),
            static_cast<float>(x_(0) + w / 2), static_cast<float>(x_(1) + h / 2)};
}

}  // namespace rearview::track
