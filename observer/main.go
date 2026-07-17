package main

/*
#include <fcntl.h>
#include <sys/mman.h>
#include <unistd.h>

// C wrapper to bypass Go's inability to call variadic functions
static int open_shm(const char *pathname, int flags) {
    return open(pathname, flags);
}
*/
import "C"
import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"unsafe"
)

const RingBufferSize = 65536

// 1. Memory structure must perfectly match C++
type TradePayload struct {
	Timestamp   int64
	Price       float64
	Volume      float64
	SeqID       uint32
	Symbol      [16]byte
	MagicNumber uint32
	Padding     [12]byte
}

type RingBuffer struct {
	WriteIndex uint64
	Payloads   [RingBufferSize]TradePayload
}

// 2. Semantic Hydration Payload
type AlertPayload struct {
	Timestamp string `json:"timestamp"`
	HexDump   string `json:"hex_dump"`
	AlertType string `json:"alert_type"`
	Context   string `json:"context"` // The hydrated English context for the LLM
}

func sendAlert(hexDump string, alertType string, contextMsg string) {
	payload := AlertPayload{
		Timestamp: time.Now().Format(time.RFC3339),
		HexDump:   hexDump,
		AlertType: alertType,
		Context:   contextMsg,
	}

	jsonData, _ := json.Marshal(payload)

	fmt.Printf("\n[Worker] Hydrating and shipping payload: %s\n", hexDump)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post("http://localhost:8000/analyze", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("[Worker] Failed to reach Python Brain:", err)
		return
	}
	defer resp.Body.Close()
	fmt.Println("[Worker] Brain successfully received the state dump.")
}

func main() {
	// 3. Connect to the C++ Shared Memory
	fd := C.open_shm(C.CString("/dev/shm/hft_ring_buffer"), C.O_RDWR)
	if fd < 0 {
		fmt.Println("Failed to open shared memory. Is the C++ engine running?")
		return
	}
	defer C.close(fd)

	size := C.size_t(unsafe.Sizeof(RingBuffer{}))
	ptr := C.mmap(nil, size, C.PROT_READ, C.MAP_SHARED, fd, 0)
	if ptr == C.MAP_FAILED {
		fmt.Println("mmap failed")
		return
	}

	ringBuffer := (*RingBuffer)(ptr)
	var localReadIndex uint64 = 0

	fmt.Println("[Spy] Observer Online. Watching shared memory...")

	// Track the timestamp of the last successful read for the Heartbeat Monitor
	lastReadTime := time.Now()

	for {
		writeIndex := ringBuffer.WriteIndex

		if localReadIndex < writeIndex {
			idx := localReadIndex % RingBufferSize
			payload := ringBuffer.Payloads[idx]

			// 1. Calculate what sequence ID we EXPECT this payload to have
			expectedSeqID := uint32(localReadIndex)

			// 2. The Resilient Lapping Check
			if payload.SeqID > expectedSeqID {
				droppedFrames := payload.SeqID - expectedSeqID
				hexStr := "0xLAPPED"
				contextMsg := fmt.Sprintf("The C++ engine lapped the observer. Dropped %d trades. Expected SeqID %d, but found SeqID %d. Resyncing...", droppedFrames, expectedSeqID, payload.SeqID)
				
				fmt.Printf("[Spy] WARNING: Missed %d trades! Resyncing to oldest available data...\n", droppedFrames)
				
				// Fire the alert gracefully in the background without halting the main thread
				go sendAlert(hexStr, "WARNING_LAPPED", contextMsg)
				
				// --- THE LATCH (RECOVERY MECHANISM) ---
				if writeIndex > RingBufferSize {
					localReadIndex = writeIndex - RingBufferSize
				} else {
					localReadIndex = 0
				}
				
				// Skip the rest of this loop iteration and immediately start reading from the newly synced position
				continue
			}

			// --- DETECTION PROTOCOL 1: CORRUPTION (True Fatal Errors) ---
			// --- DETECTION PROTOCOL 1: UNHEALTHY MAGIC NUMBER ---
			if payload.MagicNumber != 0xBEEFCAFE {
				hexStr := fmt.Sprintf("0x%X", payload.MagicNumber)
				
				// Check if this is an intercepted C++ signal death rattle (e.g., 0xDEAD000B, 0xDEAD0002)
				if payload.MagicNumber >= 0xDEAD0000 && payload.MagicNumber <= 0xDEADFFFF {
					contextMsg := fmt.Sprintf("JIRA-801 [DYING BREATH]: The C++ engine executed a dying breath OS signal handler before termination. Captured code: %s. Last known sequence: %d. ROOT CAUSE: Fatal signal caught.", hexStr, payload.SeqID)
					
					fmt.Printf("[Spy] DYING BREATH DETECTED: %s\n", hexStr)
					go sendAlert(hexStr, "DYING_BREATH_ALERT", contextMsg)
				} else {
					// True physical memory corruption / bit-flip
					contextMsg := fmt.Sprintf("The C++ trading engine encountered fatal memory corruption. Terminal exit code: %s. Last known sequence: %d.", hexStr, payload.SeqID)
					
					fmt.Printf("[Spy] CRITICAL CORRUPTION DETECTED: %s\n", hexStr)
					go sendAlert(hexStr, "CORRUPTION_ALERT", contextMsg)
				}
				
				time.Sleep(1 * time.Second)
				return
			}

			// 3. Healthy Read
			localReadIndex++
			lastReadTime = time.Now() // Reset the heartbeat

		} else {
			// --- DETECTION PROTOCOL 2: THE HEARTBEAT MONITOR ---
			// If 10 milliseconds pass without a new trade, the C++ engine is dead.
			if time.Since(lastReadTime) > 30*time.Millisecond {
				
				// Peek at the very last written slot to see if C++ left a dying breath signal
				lastWrittenIdx := (writeIndex - 1) % RingBufferSize
				lastPayload := ringBuffer.Payloads[lastWrittenIdx]

				var hexStr string
				var contextMsg string
				var alertType string

				if lastPayload.MagicNumber != 0xBEEFCAFE {
					// Found the C++ Signal Handler's dying breath
					hexStr = fmt.Sprintf("0x%X", lastPayload.MagicNumber)
					alertType = "DYING_BREATH_ALERT"
					contextMsg = fmt.Sprintf("The C++ engine executed a dying breath OS signal handler before termination. Captured code: %s.", hexStr)
					fmt.Printf("[Spy] DYING BREATH DETECTED: %s\n", hexStr)
				} else {
					// Total sudden death (e.g., OOM Kill)
					hexStr = "0xUNKNOWN"
					alertType = "SUDDEN_DEATH_ALERT"
					contextMsg = "Heartbeat lost. The C++ engine was instantly destroyed by the OS without a death rattle."
					fmt.Println("[Spy] SUDDEN DEATH DETECTED. Heartbeat flatlined.")
				}

				go sendAlert(hexStr, alertType, contextMsg)
				
				// Flight Recorder Lockdown: Stop reading and preserve the 64KB RAM state
				time.Sleep(1 * time.Second)
				return
			}
		}
	}
}