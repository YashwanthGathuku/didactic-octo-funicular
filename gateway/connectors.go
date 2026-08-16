package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/connectors"
)

// The connector platform's HTTP surface.
//
// Everything the wizard renders comes from here, so the field model, the
// authentication modes, the safe template and the status all live on the server.
// The browser holds no knowledge of any specific database, which is what makes
// the wizard generic -- and, more usefully, means a UI bug cannot expose a
// field the server intended to keep write-only.

// connectorRegistry is built once at startup.
//
// PostgreSQL is registered with the conformance record produced by this build's
// verification run, and every other entry keeps its PLANNED status. There is no
// code path that promotes an entry without a record; see connectors.Registry.
var (
	registryOnce sync.Once
	registry     *connectors.Registry
)

func connectorRegistry() *connectors.Registry {
	registryOnce.Do(func() {
		registry = connectors.NewRegistry()

		// Evidence comes from a file the conformance run produces and the
		// deployment ships, not from a constant in the binary.
		//
		// A process that has just booted has verified nothing itself, so
		// promoting a connector from a compiled-in value would be a claim of
		// verification made by the component that did not do the verifying --
		// the same defect as returning mTLSVerified: true. With no evidence
		// file the driver is registered anyway and stays IMPLEMENTING:
		// visible, and not selectable.
		evidence, err := connectors.LoadEvidence()
		if err != nil {
			// Logged and treated as absent. A deployment that meant to carry
			// evidence and shipped something broken must find out, and the
			// safe direction is the connector being unusable rather than
			// wrongly trusted.
			log.Printf("connector evidence: %v", err)
			evidence = nil
		}

		for _, note := range connectors.ApplyEvidence(registry,
			[]connectors.Connector{connectors.NewPostgresConnector()}, evidence) {
			log.Printf("connectors: %s", note)
		}
	})
	return registry
}

// registerConnectorRoutes mounts the catalog and descriptor endpoints.
func registerConnectorRoutes(r chi.Router) {
	r.Get("/connectors", listConnectors)
	r.Get("/connectors/{type}", describeConnector)
	r.Post("/connectors/{type}/parse-uri", parseConnectorURI)
}

// listConnectors returns the whole catalog, including entries with no driver.
//
// All nine are returned. Hiding the unimplemented ones would make the platform
// look complete; returning them with an honest status and a reason is what lets
// an operator tell "not built" from "broken".
func listConnectors(w http.ResponseWriter, r *http.Request) {
	if _, serr := resolveScope(r, auth.PermReadTenant); serr != nil {
		serr.write(w)
		return
	}

	catalog := connectorRegistry().Catalog()
	out := make([]map[string]any, 0, len(catalog))
	for _, d := range catalog {
		out = append(out, map[string]any{
			"type":         d.Type,
			"displayName":  d.DisplayName,
			"status":       d.Status,
			"statusReason": d.StatusReason,
			// Selectable is derived on the server and sent explicitly, so the
			// UI does not have to reimplement the rule. A client that got the
			// rule wrong would offer a connector the server then refuses,
			// which reads to the operator as a broken product rather than a
			// deliberate gate.
			"selectable":  d.Status.Selectable(),
			"conformance": d.Conformance,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"connectors": out,
		"available":  connectorRegistry().Available(),
		"note": "A connector becomes selectable only after its real driver passes the shared " +
			"conformance suite against a real server. Statuses other than AVAILABLE cannot be connected to.",
	})
}

// describeConnector returns the descriptor that drives the wizard.
func describeConnector(w http.ResponseWriter, r *http.Request) {
	if _, serr := resolveScope(r, auth.PermReadTenant); serr != nil {
		serr.write(w)
		return
	}

	d, err := connectorRegistry().Descriptor(chi.URLParam(r, "type"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error":   "unknown_connector",
			"message": "that connector is not in the catalog",
		})
		return
	}
	// The descriptor is returned whatever the status: an operator reviewing a
	// PLANNED connector's field model is a legitimate thing to do, and it
	// carries no connection and no secret.
	writeJSON(w, http.StatusOK, d)
}

// parseConnectorURI splits a pasted connection string.
//
// The raw string is never stored, logged or echoed. The response carries the
// extracted non-secret fields, a flag saying a credential was found, and any
// warnings -- and never the credential itself or the string it came from.
func parseConnectorURI(w http.ResponseWriter, r *http.Request) {
	scope, serr := resolveScope(r, auth.PermManageContract)
	if serr != nil {
		serr.write(w)
		return
	}

	d, err := connectorRegistry().Descriptor(chi.URLParam(r, "type"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown_connector"})
		return
	}

	var body struct {
		URI string `json:"uri"`
	}
	// The body is bounded: a paste box is an unbounded input from a browser.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "invalid_request",
			"message": "the request body could not be read",
		})
		return
	}

	parsed, err := connectors.ParseConnectionURI(d, body.URI)
	if err != nil {
		// The error from ParseConnectionURI never contains the input, which is
		// why it can be returned verbatim.
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "invalid_connection_string",
			"message": err.Error(),
		})
		return
	}

	// Recorded without the string. An operator pasting a connection string is
	// an event worth auditing; the string is not worth keeping.
	log.Printf("connectors: tenant %s parsed a %s connection string (credential present: %v)",
		scope.TenantID(), d.Type, parsed.HasSecret)

	writeJSON(w, http.StatusOK, map[string]any{
		"fields":          parsed.Fields,
		"secretExtracted": parsed.HasSecret,
		"warnings":        parsed.Warnings,
		"note": "The credential was separated and the pasted string discarded. It is not stored " +
			"and cannot be retrieved.",
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
