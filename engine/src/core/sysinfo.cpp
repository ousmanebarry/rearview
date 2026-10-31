#include "core/sysinfo.h"

#ifdef _WIN32
#include <windows.h>
// psapi.h needs windows.h first.
#include <psapi.h>
#else
#include <sys/resource.h>
#endif

namespace rearview {

double peak_memory_mb() {
#ifdef _WIN32
    PROCESS_MEMORY_COUNTERS pmc{};
    if (GetProcessMemoryInfo(GetCurrentProcess(), &pmc, sizeof(pmc))) {
        return static_cast<double>(pmc.PeakWorkingSetSize) / (1024.0 * 1024.0);
    }
    return 0;
#else
    rusage ru{};
    getrusage(RUSAGE_SELF, &ru);
    return static_cast<double>(ru.ru_maxrss) / 1024.0;  // KB on Linux
#endif
}

}  // namespace rearview
