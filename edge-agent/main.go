package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// EdgeAgentConfig holds configuration for the outbound-only private edge agent.
type EdgeAgentConfig struct {
	AgentID      string `json:"agentId"`
	TenantID     string `json:"tenantId"`
	ControlPlane string `json:"controlPlane"`
	CertFile     string `json:"certFile"`
	KeyFile      string `json:"keyFile"`
	CAFile       string `json:"caFile"`
	PollInterval time.Duration
}

func main() {
	controlPlaneURL := flag.String("control-plane", "https://localhost:8443", "Sentinel Flow Control Plane URL (HTTPS)")
	agentID := flag.String("agent-id", "EDGE-AGENT-MERIDIAN-VPC-01", "Unique Edge Agent Identifier")
	tenantID := flag.String("tenant-id", "TENANT-DEFAULT", "Tenant Identifier")
	certFile := flag.String("cert", "", "Path to agent X.509 client certificate")
	keyFile := flag.String("key", "", "Path to agent client private key")
	caFile := flag.String("ca", "", "Path to root/intermediate CA certificate")
	interval := flag.Int("interval", 15, "Outbound heartbeat sync interval in seconds")
	flag.Parse()

	hostname, _ := os.Hostname()
	cfg := EdgeAgentConfig{
		AgentID:      *agentID,
		TenantID:     *tenantID,
		ControlPlane: *controlPlaneURL,
		CertFile:     *certFile,
		KeyFile:      *keyFile,
		CAFile:       *caFile,
		PollInterval: time.Duration(*interval) * time.Second,
	}

	fmt.Println("==================================================================")
	fmt.Println("  SENTINEL FLOW: Customer Edge Agent (Outbound-Only Private Edge)")
	fmt.Printf("  Agent ID:       %s\n", cfg.AgentID)
	fmt.Printf("  Tenant ID:      %s\n", cfg.TenantID)
	fmt.Printf("  Control Plane:  %s\n", cfg.ControlPlane)
	fmt.Printf("  Local Hostname: %s\n", hostname)
	fmt.Println("  Inbound Ports:  NONE (Outbound TLS 1.2+ Client)")
	fmt.Println("==================================================================")

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Load mutual TLS client certificates if provided
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			log.Fatalf("[Edge Agent] Failed to load mTLS client certificate: %v", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load trusted root CA if provided
	if cfg.CAFile != "" {
		caData, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			log.Fatalf("[Edge Agent] Failed to read CA certificate: %v", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caData) {
			log.Fatalf("[Edge Agent] Failed to parse CA certificate PEM")
		}
		tlsConfig.RootCAs = pool
	}

	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        5,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	for {
		heartbeat := map[string]any{
			"agentId":   cfg.AgentID,
			"tenantId":  cfg.TenantID,
			"hostname":  hostname,
			"status":    "HEALTHY",
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		}

		payloadBytes, _ := json.Marshal(heartbeat)
		req, err := http.NewRequest("POST", cfg.ControlPlane+"/api/v1/edge/heartbeat", bytes.NewBuffer(payloadBytes))
		if err != nil {
			log.Printf("[Edge Agent] Failed to construct heartbeat request: %v", err)
		} else {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "Sentinel-Edge-Agent/1.0")

			resp, err := client.Do(req)
			if err != nil {
				log.Printf("[Edge Agent] Outbound control-plane notification: %v", err)
			} else {
				log.Printf("[Edge Agent] Outbound sync response: HTTP %d", resp.StatusCode)
				_ = resp.Body.Close()
			}
		}

		time.Sleep(cfg.PollInterval)
	}
}
