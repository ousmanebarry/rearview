#pragma once

#include <condition_variable>
#include <deque>
#include <mutex>
#include <optional>

namespace rearview {

// A bounded multi-producer multi-consumer queue. push blocks while full, pop
// blocks while empty; close() wakes everyone: push then fails and pop drains
// what is left before returning nullopt.
template <typename T>
class BoundedQueue {
public:
    explicit BoundedQueue(size_t cap) : cap_(cap) {}

    bool push(T v) {
        std::unique_lock lock(mu_);
        not_full_.wait(lock, [&] { return closed_ || q_.size() < cap_; });
        if (closed_) return false;
        q_.push_back(std::move(v));
        not_empty_.notify_one();
        return true;
    }

    std::optional<T> pop() {
        std::unique_lock lock(mu_);
        not_empty_.wait(lock, [&] { return closed_ || !q_.empty(); });
        if (q_.empty()) return std::nullopt;
        T v = std::move(q_.front());
        q_.pop_front();
        not_full_.notify_one();
        return v;
    }

    void close() {
        std::lock_guard lock(mu_);
        closed_ = true;
        not_full_.notify_all();
        not_empty_.notify_all();
    }

private:
    size_t cap_;
    std::deque<T> q_;
    bool closed_ = false;
    std::mutex mu_;
    std::condition_variable not_full_, not_empty_;
};

}  // namespace rearview
