package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	
	"metadata-crash-cart/observer/telemetry"
)

func main() {
	log.Println("[Observer] Booting Zero-Allocation Spy Engine...")

	// 1. Create the Buffered Channel (The Escape Hatch)
	// A buffer of 100 allows the hot path to drop 100 consecutive crash frames 
	// before it has to start discarding them.
	crashChannel := make(chan []byte, 100)

	// 2. Boot the Worker Pool (Domain: Heap-allocated, Network I/O)
	// We use 4 workers. Too many workers causes context-switching overhead.
	forensicAPI := "http://localhost:8000/analyze"
	telemetry.StartWorkerPool(4, crashChannel, forensicAPI)

	// 3. Attach to the C++ Shared Memory
	spy, err := telemetry.Attach()
	if err != nil {
		log.Fatalf("[Observer] FATAL: %v\n", err)
	}

	// 4. Launch the Hot Path (Domain: Zero-allocation, CPU pinned)
	go spy.Monitor(crashChannel)
	log.Println("[Observer] Locked onto /dev/shm. Polling at nanosecond latency.")

	// Graceful shutdown handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("[Observer] Shutting down...")
}