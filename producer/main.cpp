#include <iostream>
#include <csignal>
#include <cstdlib>
#include <fcntl.h>
#include <sys/mman.h>
#include <unistd.h>
#include <atomic>
#include <cstring>

// 64-byte payload (Matches the Go Spy struct)
struct TradePayload {
    int64_t timestamp;
    double price;
    double volume;
    uint32_t seq_id;
    char symbol[16];
    uint32_t magic_number;
    char padding[12]; // Pad to exactly 64 bytes
};

// The Shared Memory Ring Buffer
struct RingBuffer {
    std::atomic<uint64_t> write_index;
    TradePayload payloads[65536];
};

// Global pointer so the signal handler can reach it when everything goes wrong
RingBuffer* shm_rb = nullptr;

// --- THE DYING BREATH HANDLER ---
void death_handler(int signum) {
    if (shm_rb != nullptr) {
        uint64_t current_idx = shm_rb->write_index.load(std::memory_order_relaxed) % 65536;
        
        // Write the terminal death rattle directly into physical RAM
        shm_rb->payloads[current_idx].magic_number = 0xDEAD0000 + signum; // e.g., 0xDEAD000B for SIGSEGV
        
        // Memory barrier to ensure the Go Spy sees it instantly
        shm_rb->write_index.fetch_add(1, std::memory_order_release);
        
        std::cerr << "\n[ENGINE PANIC] Caught OS Signal " << signum << ". Death rattle written to /dev/shm. Halting.\n";
    }
    exit(signum); // Allow the process to die
}

int main() {
    // 1. Register OS Signal Intercepts
    signal(SIGSEGV, death_handler); // Catch null pointers / memory corruption
    signal(SIGABRT, death_handler); // Catch intentional aborts
    signal(SIGINT, death_handler);  // Catch Ctrl+C for testing

    // 2. Map POSIX Shared Memory
    int fd = shm_open("/hft_ring_buffer", O_CREAT | O_RDWR, 0666);
    ftruncate(fd, sizeof(RingBuffer));
    shm_rb = (RingBuffer*)mmap(0, sizeof(RingBuffer), PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);

    std::cout << "[Producer] Engine Online. Signals armed. Blasting trades...\n";

    // 3. The Unthrottled Hot Path
    uint64_t local_seq = 0;
    while (true) {
        uint64_t idx = shm_rb->write_index.load(std::memory_order_relaxed) % 65536;
        
        shm_rb->payloads[idx].timestamp = 1700000000000 + local_seq;
        shm_rb->payloads[idx].price = 150.25;
        shm_rb->payloads[idx].volume = 100.0;
        shm_rb->payloads[idx].seq_id = local_seq;
        std::strncpy(shm_rb->payloads[idx].symbol, "AAPL", 16);
        
        // Healthy magic number
        shm_rb->payloads[idx].magic_number = 0xBEEFCAFE; 

        // Atomic release
        shm_rb->write_index.fetch_add(1, std::memory_order_release);
        local_seq++;

        // --- THE MICRO-BATCH THROTTLE (100k TPS) ---
        // Burst 2,000 trades, then sleep 20ms to bypass WSL tick rounding
        if (local_seq % 2000 == 0) {
            usleep(20000); 
        }
    }

    return 0;
}