#pragma once
#include <cstdint>
#include <atomic>
#include <cstddef>

// 65536 is 2^16. Mask is 65535 (0xFFFF).
constexpr size_t RING_SIZE = 65536;
constexpr size_t RING_MASK = RING_SIZE - 1;
constexpr uint32_t MAGIC_NUMBER = 0xBEEFCAFE;

// Align to 64 bytes to match physical CPU cache lines.
// 8 (seq_id) + 8 (price) + 4 (volume) + 4 (magic) = 24 bytes.
// We pad the remaining 40 bytes to hit the 64-byte boundary.
struct alignas(64) TradePayload {
    uint64_t seq_id;
    double price;
    uint32_t volume;
    uint32_t magic;
    char padding[40]; 
};

// The shared memory layout. 
// write_index is aligned to its own 64-byte boundary to prevent false sharing
// with the slots array that follows it.
struct alignas(64) SharedRingBuffer {
    std::atomic<uint64_t> write_index;
    char index_padding[56]; // 8 bytes for atomic uint64_t + 56 = 64 bytes

    TradePayload slots[RING_SIZE];
};