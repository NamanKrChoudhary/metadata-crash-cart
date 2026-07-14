package telemetry

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// CrashReport is the JSON structure we send to the Python Brain
type CrashReport struct {
	Timestamp string `json:"timestamp"`
	HexDump   string `json:"hex_dump"`
	AlertType string `json:"alert_type"`
}

// StartWorkerPool initializes a fixed number of goroutines sleeping on the channel.
func StartWorkerPool(workerCount int, crashChannel <-chan []byte, forensicAPI string) {
	// Pre-configure an HTTP client with connection pooling and strict timeouts.
	// We don't want workers hanging indefinitely if Python crashes.
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  true, // Save CPU, payloads are small
		},
	}

	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			log.Printf("[Worker %d] Booted and sleeping on channel...", workerID)
			
			// The worker blocks here with zero CPU usage until the hot path pushes data.
			for rawDump := range crashChannel {
				processDump(workerID, rawDump, client, forensicAPI)
			}
		}(i)
	}
}

func processDump(workerID int, rawDump []byte, client *http.Client, targetURL string) {
	// 1. Format the raw bytes into a hex string
	hexString := hex.EncodeToString(rawDump)

	// 2. Build the JSON payload (Heap allocation happens here)
	report := CrashReport{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		HexDump:   hexString,
		AlertType: "MEMORY_CORRUPTION_OR_OVERRUN",
	}

	jsonData, err := json.Marshal(report)
	if err != nil {
		log.Printf("[Worker %d] JSON Marshal failed: %v", workerID, err)
		return
	}

	// 3. Execute the network I/O
	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("[Worker %d] Request creation failed: %v", workerID, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[Worker %d] Target Unreachable (Is Python running?): %v", workerID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Printf("[Worker %d] Successfully fired RCA trigger for crash payload.", workerID)
	} else {
		log.Printf("[Worker %d] Python API rejected payload. Status: %d", workerID, resp.StatusCode)
	}
}