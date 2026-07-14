package telemetry

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	RingSize    = 65536
	RingMask    = RingSize - 1
	MagicNumber = 0xBEEFCAFE
	ShmPath     = "/dev/shm/hft_telemetry_ring"
)

// TradePayload strictly mirrors the 64-byte C++ struct.
type TradePayload struct {
	SeqID   uint64
	Price   float64
	Volume  uint32
	Magic   uint32
	Padding [40]byte
}

// SharedRingBuffer strictly mirrors the C++ memory layout.
type SharedRingBuffer struct {
	WriteIndex   uint64
	IndexPadding [56]byte
	Slots        [RingSize]TradePayload
}

// SpyEngine holds the pointer to the mapped memory.
type SpyEngine struct {
	memory []byte
	ring   *SharedRingBuffer
}

// Attach connects to the POSIX shared memory created by C++.
func Attach() (*SpyEngine, error) {
	file, err := os.OpenFile(ShmPath, os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open SHM (is producer running?): %v", err)
	}
	defer file.Close()

	// Retrieve file size to map exactly what C++ allocated
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// The syscall.Mmap acts as our bridge. MAP_SHARED ensures we see C++ writes.
	mmapData, err := syscall.Mmap(int(file.Fd()), 0, int(stat.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmap failed: %v", err)
	}

	// The unsafe.Pointer cast. We tell the Go runtime to treat raw bytes as our struct.
	// This takes 0 CPU cycles—it is purely a compiler directive.
	ringPtr := (*SharedRingBuffer)(unsafe.Pointer(&mmapData[0]))

	return &SpyEngine{
		memory: mmapData,
		ring:   ringPtr,
	}, nil
}

// Monitor executes the zero-allocation hot path.
func (s *SpyEngine) Monitor(crashChannel chan<- []byte) {
	// 1. Thread Pinning: Force Go to keep this goroutine on the current OS thread.
	// If the Go scheduler migrated this routine to another CPU core, we would lose our L1 cache locality.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var localReadIndex uint64 = 1

	for {
		// 2. Atomic Acquire: Hardware memory barrier preventing read reordering.
		// We only read memory that C++ has officially committed.
		writeIndex := atomic.LoadUint64(&s.ring.WriteIndex)

		// THE FIX: Lap Detection & Recovery Jump
		// If the C++ producer has outpaced our reader by more than the capacity of the ring,
		// it means old slots are actively being overwritten. Processing them causes false crash reports.
		if writeIndex >= localReadIndex && (writeIndex-localReadIndex) >= RingSize {
			// We have been lapped. Snap the read index forward to the oldest surviving packet.
			// This prevents a death loop and skips missing sequences gracefully.
			localReadIndex = writeIndex - RingSize + 1
		}

		for localReadIndex <= writeIndex {
			slotIndex := localReadIndex & RingMask
			
			// Zero-allocation pointer arithmetic to read the payload
			payload := &s.ring.Slots[slotIndex]

			// Crash detection physics:
			// If the sequence ID doesn't match, or the magic number is corrupted,
			// the C++ engine has overrun the buffer or written garbled memory.
			if payload.SeqID != localReadIndex || payload.Magic != MagicNumber {
				// We detected a crash. Slice out a chunk of the raw memory for forensics.
				// This is the ONLY allocation allowed, and it happens strictly during a fatal error.
				crashDump := make([]byte, 1024) 
				copy(crashDump, s.memory[0:1024]) // Grab the first KB (headers + initial slots)
				
				// Non-blocking drop to the worker pool.
				select {
				case crashChannel <- crashDump:
				default:
					// Channel full. We drop it to maintain zero-blocking overhead on the hot path.
				}
			}

			localReadIndex++
		}
		
		// In a true HFT system, we would pause with an x86 `PAUSE` instruction here 
		// via assembly to yield the ALU while keeping the CPU core hot, but Go's scheduler 
		// requires runtime.Gosched() to avoid starving the garbage collector entirely.
		runtime.Gosched()
	}
}