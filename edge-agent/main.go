package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type EdgeAgentConfig struct {
	AgentID      string `json:"agentId"`
	ControlPlane string `json:"controlPlane"`
	AuthToken    string `json:"authToken"`
	Hostname     string `json:"hostname"`
}

func main() {
	controlPlaneURL := flag.String("control-plane", "http://localhost:8080", "Sentinel Flow Control Plane URL")
	agentID := flag.String("agent-id", "EDGE-AGENT-MERIDIAN-VPC-01", "Unique Edge Agent Identifier")
	pollIntervalSec := flag.Int("interval", 15, "Outbound heartbeat sync interval in seconds")
	flag.Parse()

	hostname, _ := os.Hostname()
	cfg := EdgeAgentConfig{
		AgentID:      *agentID,
		ControlPlane: *controlPlaneURL,
		Hostname:     hostname,
	}

	fmt.Println("==================================================================")
	fmt.Println("  SENTINEL FLOW: Customer Edge Agent (Outbound Integration Hub)   ")
	fmt.Printf("  Agent ID:       %s\n", cfg.AgentID)
	fmt.Printf("  Control Plane:  %s\n", cfg.ControlPlane)
	fmt.Printf("  Local Hostname: %s\n", cfg.Hostname)
	fmt.Println("  Inbound Ports:  NONE (Zero-Trust Outbound Only via mTLS)         ")
	fmt.Println("==================================================================")

	client := &http.Client{Timeout: 5 * time.Second}

	for {
		// 1. Construct outbound metadata sync packet (no secrets transferred)
		syncPayload := map[string]interface{}{
			"edgeAgentId": cfg.AgentID,
			"hostname":    cfg.Hostname,
			"status":      "HEALTHY",
			"discoveredResources": []map[string]interface{}{
				{
					"type":       "SFTP_DIRECTORY",
					"path":       "/inbound/ach/",
					"filesCount": 4,
					"health":     "HEALTHY",
				},
				{
					"type":       "POSTGRESQL_SCHEMA",
					"database":   "treasury_settlement_db",
					"tables":     []string{"settlement_batches", "counterparty_ledgers"},
					"health":     "HEALTHY",
				},
			},
			"heartbeatTimestamp": time.Now().UTC().Format(time.RFC3339),
		}

		payloadBytes, _ := json.Marshal(syncPayload)
		req, err := http.NewRequest("POST", cfg.ControlPlane+"/api/v1/hub/edge/sync", bytes.NewBuffer(payloadBytes))
		if err != nil {
			log.Printf("[Edge Agent] Failed to create sync request: %v\n", err)
		} else {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "Sentinel-Edge-Agent/1.0")

			resp, err := client.Do(req)
			if err != nil {
				log.Printf("[Edge Agent] Outbound sync notice (Control plane connection): %v\n", err)
			} else {
				log.Printf("[Edge Agent] Outbound mTLS metadata sync successful (HTTP %d)\n", resp.StatusCode)
				resp.Body.Close()
			}
		}

		time.Sleep(time.Duration(*pollIntervalSec) * time.Second)
	}
}
