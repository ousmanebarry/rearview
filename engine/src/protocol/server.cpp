#include "protocol/server.h"

#include <condition_variable>
#include <iostream>
#include <new>
#include <thread>

namespace rearview::protocol {

void Emitter::send(const Json& msg) {
    const std::string line = msg.dump() + "\n";
    std::lock_guard lock(mu_);
    out_ << line;
    out_.flush();
}

Server::Server(std::istream& in, std::ostream& out, ServerInfo info,
               std::map<std::string, Handler> handlers)
    : in_(in), emit_(out), info_(std::move(info)), handlers_(std::move(handlers)) {}

void Server::reader() {
    std::string line;
    while (std::getline(in_, line)) {
        if (!line.empty() && line.back() == '\r') line.pop_back();
        if (line.empty()) continue;
        Json cmd;
        try {
            cmd = Json::parse(line);
        } catch (const std::exception& e) {
            emit_.send(error_json("bad_request", std::string("invalid JSON: ") + e.what()));
            continue;
        }
        if (!cmd.is_object() || !cmd.contains("id") || !cmd["id"].is_number_integer() ||
            !cmd.contains("cmd") || !cmd["cmd"].is_string()) {
            Json err = error_json("bad_request", "a command needs an integer id and a cmd");
            if (cmd.is_object() && cmd.contains("id")) err["id"] = cmd["id"];
            emit_.send(err);
            continue;
        }
        const std::string name = cmd["cmd"];
        std::lock_guard lock(mu_);
        if (name == "cancel") {
            const long long target = cmd.value("target_id", -1LL);
            cancelled_.insert(target);
            if (target == running_) cancel_ = true;
            continue;
        }
        if (name == "shutdown") {
            if (running_ != -1) cancel_ = true;  // stop the running job so we exit promptly
            closed_ = true;
            cv_.notify_all();
            return;
        }
        queue_.push_back(std::move(cmd));
        cv_.notify_all();
    }
    // End of input: finish what was queued (nobody can cancel it any more), then exit.
    std::lock_guard lock(mu_);
    closed_ = true;
    cv_.notify_all();
}

void Server::execute(const Json& cmd) {
    const long long id = cmd["id"];
    JobEmitter je(emit_, id);
    const std::string name = cmd["cmd"];
    if (name == "ping") {
        je.send({{"type", "pong"}});
        return;
    }
    auto it = handlers_.find(name);
    if (it == handlers_.end()) {
        je.send(error_json("unknown_command", "unknown command: " + name));
        return;
    }
    try {
        je.send(it->second(cmd, je, cancel_));
    } catch (const pipeline::AnalyzeError& e) {
        je.send(error_json(e.code, e.what(), e.t));
    } catch (const std::bad_alloc&) {
        je.send(error_json("out_of_memory", "out of memory"));
    } catch (const nlohmann::json::exception& e) {
        je.send(error_json("bad_request", e.what()));
    } catch (const std::exception& e) {
        je.send(error_json("internal", e.what()));
    }
}

int Server::run() {
    emit_.send({{"type", "ready"},
                {"protocol_version", kProtocolVersion},
                {"engine_version", info_.engine_version},
                {"device", info_.device},
                {"capabilities", info_.capabilities}});
    std::thread t(&Server::reader, this);
    while (true) {
        Json cmd;
        {
            std::unique_lock lock(mu_);
            cv_.wait(lock, [&] { return !queue_.empty() || closed_; });
            if (queue_.empty()) break;  // closed and nothing left
            cmd = std::move(queue_.front());
            queue_.erase(queue_.begin());
            running_ = cmd["id"];
            cancel_ = cancelled_.count(running_) > 0;
        }
        execute(cmd);
        std::lock_guard lock(mu_);
        cancelled_.erase(running_);
        running_ = -1;
    }
    // The reader blocks on stdin; on shutdown it has already returned, on EOF too.
    t.join();
    return 0;
}

}  // namespace rearview::protocol
