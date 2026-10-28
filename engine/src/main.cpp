#include <format>
#include <iostream>

#include "decode/probe.h"

int main(int argc, char** argv) {
    if (argc != 2) {
        std::cerr << "usage: engine <video file>\n";
        return 2;
    }
    const auto info = rearview::decode::probe(argv[1]);
    if (!info) {
        std::cerr << "engine: cannot open video: " << argv[1] << "\n";
        return 1;
    }
    std::cout << std::format("{}x{} {:.2f} fps {} frames\n", info->width, info->height, info->fps,
                             info->frame_count);
    return 0;
}
