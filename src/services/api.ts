/**
 * The connector platform's client.
 *
 * This file used to carry its own `request()`, its own `ApiResult`, and its own
 * base URL -- a second transport alongside `src/api/client.ts` with its own
 * ideas about credentials, the tenant selector and CSRF. Two transports means
 * two places to fix anything, and the one that gets fixed is the one somebody
 * remembered. It now re-exports the single client and holds only the connector
 * and connection *types* and calls.
 */

import { ApiResult, request } from '../api/client';

export type { ApiResult };

/**
 * What ingestion returns.
 *
 * The only shape from this block that survives. Everything that used to sit
 * alongside it -- checkHealth, getSlaBoard, getIncidents, getLedger,
 * triggerTriage, approveIncident, triggerChaos, runBenchmark, runEvals,
 * getComplianceExport -- was either replaced by a typed call in
 * `src/api/endpoints.ts` or pointed at a route Prompt 01 deleted. Three of them
 * (`/chaos/trigger`, `/benchmark/run`, and the incident triage the fabricated
 * analyst fell back to) would have 404'd if anything had called them, and
 * because each threw on failure, the caller could not tell a missing route from
 * an outage.
 */
export interface ApiIngestionResult {
  fileId: number;
  filename: string;
  hash: string;
  sizeBytes: number;
  // Terminus of ingestion. RELEASED is NOT produced here: release requires a
  // policy decision and approval, which ingestion does not perform.
  status: 'QUARANTINED' | 'VALIDATED';
  totalRecordsParsed: number;
  totalEntriesParsed: number;

  // Integer minor units. The totalDebitsUsd/totalCreditsUsd fields these
  // replace were floats computed as cents/100, and the UI compared them for
  // equality to render a balance badge.
  totalDebitsMinor: number;
  totalCreditsMinor: number;

  // isBalanced is deliberately gone. Whether a file must balance is a term of
  // the feed contract, not a property of the file: a credit-only payroll file
  // never balances and is entirely correct.

  // What produced the status. A verdict without these is an opinion.
  policyVersion: string;
  contractId?: string;

  // Rules the validator could not evaluate for lack of an authoritative
  // source. The UI shows this so silence does not imply coverage.
  notCheckedRuleIds?: string[];
  quarantineReasons?: string[];

  findings: {
    id: number;
    code: string;
    ruleVersion: string;
    provenance: string;
    description: string;
    severity: 'INFO' | 'WARNING' | 'BLOCKING';
    lineNumber: number;
    byteOffset: number;
    fieldStart?: number;
    fieldEnd?: number;
    // Redacted at the server, at the point it is produced. There is no raw
    // record content in this response.
    evidence?: string;
    expected?: string;
    actual?: string;
  }[];
  incidentId?: number;
}

/**
 * Ingestion, through the shared transport.
 *
 * It returns an ApiResult like everything else rather than throwing. The
 * previous form threw an Error carrying the response text, so a caller had to
 * parse a string to tell "the gateway is down" from "this file was rejected"
 * -- and every caller that forgot took down the component tree.
 */
export async function ingestRawNacha(
  filename: string,
  content: string,
): Promise<ApiResult<ApiIngestionResult>> {
  return request<ApiIngestionResult>('/files/ingest-raw', {
    method: 'POST',
    body: { filename, content },
  });
}

// ---------------------------------------------------------------------------
// Connector platform
// ---------------------------------------------------------------------------

/**
 * The connector catalog and its descriptors are server-owned.
 *
 * Nothing here knows what a PostgreSQL connection needs, or an Oracle one. The
 * wizard renders whatever fields the descriptor lists, so adding a connector is
 * a server change and there is no second place where "Oracle needs a service
 * name" has to be remembered -- and no client-side field list that can drift out
 * of step with what the server will accept.
 */

export type ConnectorStatus =
  | 'PLANNED'
  | 'IMPLEMENTING'
  | 'AVAILABLE'
  | 'DEGRADED'
  | 'DISABLED';

export interface ConnectorCatalogEntry {
  type: string;
  displayName: string;
  status: ConnectorStatus;
  statusReason: string;
  /**
   * Decided by the server, not re-derived here. A client that reimplemented the
   * rule and got it wrong would offer a connector the server then refuses,
   * which reads to an operator as a broken product rather than a deliberate
   * gate.
   */
  selectable: boolean;
  conformance: ConnectorConformance | null;
}

export interface ConnectorConformance {
  connectorType: string;
  serverVersion: string;
  driverVersion: string;
  testCommit: string;
  runAt: string;
  passed: number;
  failed: number;
  skipped?: string[];
}

export type ConnectorFieldKind =
  | 'TEXT'
  | 'NUMBER'
  | 'ENUM'
  | 'BOOL'
  | 'SECRET'
  | 'SECRET_REF'
  | 'LIST';

export interface ConnectorFieldOption {
  value: string;
  label: string;
  help?: string;
  /** Weakens a control. The wizard warns rather than hiding it. */
  insecure?: boolean;
}

export interface ConnectorField {
  id: string;
  label: string;
  kind: ConnectorFieldKind;
  required: boolean;
  help?: string;
  placeholder?: string;
  default?: string;
  options?: ConnectorFieldOption[];
  rule?: string;
  /** Restricts the field to particular authentication modes. */
  appliesToAuth?: string[];
  sensitive?: boolean;
}

export interface ConnectorAuthMode {
  id: string;
  label: string;
  help?: string;
  preferred?: boolean;
  localTestingOnly?: boolean;
}

export interface ConnectorDescriptor {
  type: string;
  displayName: string;
  status: ConnectorStatus;
  statusReason?: string;
  fields: ConnectorField[];
  authModes: ConnectorAuthMode[];
  /** A placeholder template shown as documentation. Never a saved value. */
  template?: string;
  supportsUriPaste: boolean;
  capabilities: Record<string, unknown>;
  conformance?: ConnectorConformance | null;
}

export interface ConnectorCatalog {
  connectors: ConnectorCatalogEntry[];
  available: string[];
  note: string;
}

export async function getConnectorCatalog(): Promise<ApiResult<ConnectorCatalog>> {
  return request<ConnectorCatalog>(`/connectors`);
}

export async function getConnectorDescriptor(
  type: string,
): Promise<ApiResult<ConnectorDescriptor>> {
  return request<ConnectorDescriptor>(`/connectors/${encodeURIComponent(type)}`);
}

export interface ParsedConnectionUri {
  fields: Record<string, string>;
  secretExtracted: boolean;
  warnings: string[] | null;
  note: string;
}

/**
 * Sends a pasted connection string to the server, which splits it and discards
 * the raw value.
 *
 * Parsing happens on the server rather than in the browser so the credential is
 * separated by the component that owns the secret store. A browser-side parse
 * would leave the whole string in a React state variable, in the form's DOM, and
 * in whatever the next error boundary decides to log.
 */
export async function parseConnectionUri(
  type: string,
  uri: string,
): Promise<ApiResult<ParsedConnectionUri>> {
  return request<ParsedConnectionUri>(
    `/connectors/${encodeURIComponent(type)}/parse-uri`,
    { method: 'POST', body: { uri } },
  );
}

// ---------------------------------------------------------------------------
// Saved source connections
// ---------------------------------------------------------------------------

/**
 * A saved connection, as every read path returns it.
 *
 * There is no credential field and no connection string, and there is no
 * client-side masking either — the server does not send one, so there is
 * nothing here to forget to mask. `secretsConfigured` names which credentials
 * exist, by field id.
 */
export interface SourceConnection {
  id: number;
  connectorType: string;
  displayName: string;
  authMode: string;
  fields: Record<string, string>;
  resourceAllowlist: string[];
  maxPerMinute: number;
  secretsConfigured: string[];
  /** Credentials the customer chose below this platform's floor. */
  weakSecrets?: string[];
  health: ConnectionHealth;
  conformanceCommit?: string;
  conformanceServer?: string;
  lastUsedAt?: string;
  createdBy: string;
  createdAt: string;
}

/**
 * NEVER_CHECKED is a distinct state from healthy and must render as such. A
 * connection nobody has tested showing green is the defect the whole connector
 * platform exists to prevent.
 */
export type ConnectionHealthState = 'NEVER_CHECKED' | 'HEALTHY' | 'DEGRADED' | 'FAILED';

export interface ConnectionHealth {
  State: ConnectionHealthState;
  CheckedAt: string;
  ErrorCategory: string;
  Detail: string;
  Latency: number;
}

export interface ConnectionTestResult {
  state: ConnectionHealthState;
  checkedAt: string;
  latencyMs: number;
  /** A sanitized classification, never the database driver's own message. */
  errorClass?: string;
  detail?: string;
}

export async function getConnections(): Promise<ApiResult<{ connections: SourceConnection[] }>> {
  return request<{ connections: SourceConnection[] }>(`/connections`);
}

export async function testConnection(id: number): Promise<ApiResult<ConnectionTestResult>> {
  return request<ConnectionTestResult>(`/connections/${id}/test`, {
    method: 'POST',
  });
}

/**
 * Replacing is the only way to change a stored credential.
 *
 * There is no update that takes the current value, because nothing can read the
 * current value — not this client, not the server's own read paths.
 */
export async function replaceConnectionSecret(
  id: number,
  field: string,
  value: string,
): Promise<ApiResult<null>> {
  return request<null>(`/connections/${id}/secrets/${encodeURIComponent(field)}`, {
    method: 'POST',
    body: { value },
  });
}

export async function deleteConnection(id: number): Promise<ApiResult<null>> {
  return request<null>(`/connections/${id}`, { method: 'DELETE' });
}


/**
 * Creates a saved connection.
 *
 * The gap this closes was recorded in CONNECTOR_PLATFORM.md: the wizard
 * collected every field the descriptor named and had nowhere to send them, so a
 * connection could only be created through the API. A wizard that gathers
 * credentials and then cannot save them is worse than no wizard -- the operator
 * has typed a password into a form for nothing, and it is now in the browser's
 * memory and possibly its autofill.
 *
 * Secrets travel in the same request as the rest of the fields and are named by
 * field id. The server routes them to the secret store before its own
 * transaction opens and never writes them to the connector tables; nothing in
 * any read path can return them afterwards, which is why there is no
 * corresponding update-with-current-value call.
 */
export interface CreateConnectionRequest {
  connectorType: string;
  displayName: string;
  authMode: string;
  fields: Record<string, string>;
  /** Keyed by field id. Write-only, in every direction. */
  secrets?: Record<string, string>;
  resourceAllowlist?: string[];
  maxPerMinute?: number;
}

export async function createConnection(
  req: CreateConnectionRequest,
): Promise<ApiResult<SourceConnection>> {
  return request<SourceConnection>('/connections', { method: 'POST', body: req });
}
