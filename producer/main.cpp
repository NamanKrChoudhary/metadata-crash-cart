#include "ring_buffer.hpp"
#include <iostream>
#include <fcntl.h>
#include <sys/mman.h>
#include <unistd.h>
#include <chrono>
#include <thread>
#include <cmath>
#include <cstring>

enum class Mode {
    MICRO_BATCH, // Mode A
    BENCHMARK,   // Mode B
    DEATH_TEST   // Mode C
};

// Dummy math to simulate 1.5us risk check
void simulate_risk_check() {
    volatile double dummy = 1.0;
    for (int i = 0; i < 200; ++i) {
        dummy = std::sin(dummy + i);
    }
}

int main(int argc, char* argv[]) {
    Mode run_mode = Mode::MICRO_BATCH;
    if (argc > 1) {
        if (std::string(argv[1]) == "benchmark") run_mode = Mode::BENCHMARK;
        if (std::string(argv[1]) == "death") run_mode = Mode::DEATH_TEST;
    }

    // 1. Setup POSIX Shared Memory
    const char* shm_name = "/hft_telemetry_ring";
    int shm_fd = shm_open(shm_name, O_CREAT | O_RDWR, 0666);
    if (shm_fd == -1) {
        perror("shm_open failed");
        return 1;
    }

    size_t shm_size = sizeof(SharedRingBuffer);
    if (ftruncate(shm_fd, shm_size) == -1) {
        perror("ftruncate failed");
        return 1;
    }

    // Map memory. MAP_SHARED ensures Go can read it.
    void* map_ptr = mmap(0, shm_size, PROT_READ | PROT_WRITE, MAP_SHARED, shm_fd, 0);
    if (map_ptr == MAP_FAILED) {
        perror("mmap failed");
        return 1;
    }

    SharedRingBuffer* ring = new (map_ptr) SharedRingBuffer();
    ring->write_index.store(0, std::memory_order_relaxed);

    std::cout << "[Producer] SHM Initialized. Mode: " << (int)run_mode << std::endl;
    std::cout << "[Producer] Buffer size: " << shm_size << " bytes." << std::endl;

    uint64_t current_seq = 1;

    // 2. The Engine Loop
    while (true) {
        uint64_t index = current_seq & RING_MASK; // 1-cycle bitwise mask
        
        TradePayload& slot = ring->slots[index];
        slot.seq_id = current_seq;
        slot.price = 150.25 + (current_seq % 100);
        slot.volume = 100;
        slot.magic = MAGIC_NUMBER;

        // Release semantics ensure the payload writes hit L1/L2 BEFORE the index updates
        ring->write_index.store(current_seq, std::memory_order_release);

        current_seq++;

        // 3. Execution Domains
        if (run_mode == Mode::MICRO_BATCH) {
            if (current_seq % 1000 == 0) {
                // Sleep 10ms to bypass OS scheduler limitations while maintaining 100k TPS
                std::this_thread::sleep_for(std::chrono::milliseconds(10));
            }
        } 
        else if (run_mode == Mode::BENCHMARK) {
            simulate_risk_check(); // 1.5us latency injection, no yielding
        }
        else if (run_mode == Mode::DEATH_TEST) {
            // Unthrottled. Intentional buffer overrun.
            // Do absolutely nothing. Just burn the CPU to lap the Go spy.
        }
    }

    return 0;
}