#pragma once

#include <atomic>
#include <condition_variable>
#include <functional>
#include <iosfwd>
#include <map>
#include <mutex>
#include <set>
#include <string>
#include <vector>

#include "protocol/messages.h"

namespace rearview::protocol {

// Writes protocol messages, one per line, flushed, safe to call from any thread.
class Emitter {
public:
    explicit Emitter(std::ostream& out) : out_(out) {}
    void send(const Json& msg);

private:
    std::ostream& out_;
    std::mutex mu_;
};

// Emits replies for one job, adding its id.
class JobEmitter {
public:
    JobEmitter(Emitter& e, long long id) : e_(e), id_(id) {}
    void send(Json msg) {
        Json out{{"id", id_}};
        for (auto& [k, v] : msg.items()) out[k] = std::move(v);
        e_.send(out);
    }
    long long id() const { return id_; }

private:
    Emitter& e_;
    long long id_;
};

// Runs one command. Streams intermediate replies (progress, frame, embedding)
// through `emit` and returns the final `done` reply. Throws
// pipeline::AnalyzeError for protocol errors.
using Handler = std::function<Json(const Json& cmd, JobEmitter& emit, const std::atomic<bool>& cancel)>;

struct ServerInfo {
    std::string engine_version;
    std::string device;
    std::vector<std::string> capabilities;
};

// The engine side of the protocol: sends `ready`, reads one command per line,
// runs jobs one at a time, and handles `cancel` on a separate reader thread so
// a long job can be stopped.
class Server {
public:
    Server(std::istream& in, std::ostream& out, ServerInfo info, std::map<std::string, Handler> handlers);
    // Returns when `shutdown` arrives or stdin closes.
    int run();

private:
    void reader();
    void execute(const Json& cmd);

    std::istream& in_;
    Emitter emit_;
    ServerInfo info_;
    std::map<std::string, Handler> handlers_;

    std::mutex mu_;
    std::vector<Json> queue_;
    bool closed_ = false;
    std::set<long long> cancelled_;  // ids cancelled before or while running
    long long running_ = -1;
    std::atomic<bool> cancel_{false};
    std::condition_variable_any cv_;
};

}  // namespace rearview::protocol
