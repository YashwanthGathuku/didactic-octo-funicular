/**
 * The gateway's routes, typed.
 *
 * One function per route, named for what it does rather than for its verb, and
 * every one returning ApiResult. Nothing here throws and nothing here returns
 * null on failure -- those were the two conventions that let an outage render
 * as a healthy empty screen.
 */

import { ApiResult, getPage, query, request } from './client';
import type {
  ArtifactDetail,
  ArtifactSummary,
  Contract,
  ContractVersion,
  Decision,
  EvidenceEntry,
  Expectation,
  Incident,
  LensDataset,
  LensInvestigation,
  LensInvestigationNode,
  LensQueryIntent,
  LensQueryResult,
  OverrideRecord,
  Page,
  ReleasePolicy,
  ServiceHealth,
  Session,
} from './types';

// ---------------------------------------------------------------------------
// Session and health
// ---------------------------------------------------------------------------

export const getSession = (signal?: AbortSignal): Promise<ApiResult<Session>> =>
  request<Session>('/session', { signal });

export const getServiceHealth = (signal?: AbortSignal): Promise<ApiResult<ServiceHealth>> =>
  request<ServiceHealth>('/service-health', { signal });

// ---------------------------------------------------------------------------
// The feed/status board
// ---------------------------------------------------------------------------

export interface BoardFilter {
  status?: string;
  cursor?: string;
  limit?: number;
}

export const getBoard = (
  f: BoardFilter = {},
  signal?: AbortSignal,
): Promise<ApiResult<Page<Expectation>>> => getPage<Expectation>('/sla-board', { ...f }, signal);

// ---------------------------------------------------------------------------
// Artifacts
// ---------------------------------------------------------------------------

export interface ArtifactFilter {
  /** One or more statuses, comma-separated. An unknown value is a 400. */
  status?: string;
  filename?: string;
  cursor?: string;
  limit?: number;
}

export const getArtifacts = (
  f: ArtifactFilter = {},
  signal?: AbortSignal,
): Promise<ApiResult<Page<ArtifactSummary>>> =>
  getPage<ArtifactSummary>('/artifacts', { ...f }, signal);

export const getArtifact = (
  id: number,
  signal?: AbortSignal,
): Promise<ApiResult<ArtifactDetail>> => request<ArtifactDetail>(`/artifacts/${id}`, { signal });

// ---------------------------------------------------------------------------
// Incidents
// ---------------------------------------------------------------------------

export const getIncidents = (
  f: { status?: string; cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
): Promise<ApiResult<Page<Incident>>> => getPage<Incident>('/incidents', { ...f }, signal);

// ---------------------------------------------------------------------------
// Review and release
// ---------------------------------------------------------------------------

export const getReviewQueue = (
  f: { cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
): Promise<ApiResult<Page<Decision>>> => getPage<Decision>('/review-queue', { ...f }, signal);

/**
 * Records a vote. The actor is never sent.
 *
 * Identity comes from the verified principal on the server and from nowhere
 * else; a client that sent an actor would have it ignored. Sending one anyway
 * would suggest to the next reader of this file that it mattered.
 */
export const voteOnDecision = (
  id: number,
  approve: boolean,
  reason: string,
): Promise<ApiResult<Decision>> =>
  request<Decision>(`/decisions/${id}/${approve ? 'approve' : 'reject'}`, {
    method: 'POST',
    body: { reason },
  });

export const releaseDecision = (id: number): Promise<ApiResult<Decision>> =>
  request<Decision>(`/decisions/${id}/release`, { method: 'POST', body: {} });

export const overrideRelease = (
  id: number,
  reason: string,
): Promise<ApiResult<Decision>> =>
  request<Decision>(`/decisions/${id}/override`, { method: 'POST', body: { reason } });

export const getReleasePolicy = (
  signal?: AbortSignal,
): Promise<ApiResult<ReleasePolicy>> => request<ReleasePolicy>('/release-policy', { signal });

export const setReleasePolicy = (p: ReleasePolicy): Promise<ApiResult<ReleasePolicy>> =>
  request<ReleasePolicy>('/release-policy', { method: 'PUT', body: p });

export const getOverrides = (
  f: { cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
): Promise<ApiResult<Page<OverrideRecord>>> =>
  getPage<OverrideRecord>('/release-overrides', { ...f }, signal);

// ---------------------------------------------------------------------------
// Contracts
// ---------------------------------------------------------------------------

export const getContracts = (signal?: AbortSignal): Promise<ApiResult<Contract[]>> =>
  request<Contract[]>('/contracts', { signal });

export const getContractVersions = (
  contractId: number | string,
  signal?: AbortSignal,
): Promise<ApiResult<{ contractId: string; versions: ContractVersion[] }>> =>
  request<{ contractId: string; versions: ContractVersion[] }>(
    `/contracts/${encodeURIComponent(String(contractId))}/versions`,
    { signal },
  );

// ---------------------------------------------------------------------------
// Evidence
// ---------------------------------------------------------------------------

export const getEvidence = (
  f: { eventType?: string; correlationId?: string; cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
): Promise<ApiResult<Page<EvidenceEntry>>> => getPage<EvidenceEntry>('/evidence', { ...f }, signal);

/**
 * The whole-chain read, which also verifies the hash links.
 *
 * Separate from the paged timeline because a page of a chain proves nothing
 * about the chain: verification needs every link. Two endpoints answering two
 * questions -- "show me what happened" and "is the record intact" -- rather
 * than one that implies the second while doing the first.
 */
export const verifyLedger = (
  signal?: AbortSignal,
): Promise<ApiResult<{ totalEvents: number; isChainValid: boolean; lastEventHash: string }>> =>
  request('/ledger', { signal });

// ---------------------------------------------------------------------------
// Ingestion
// ---------------------------------------------------------------------------

export interface IngestionResult {
  fileId: number;
  filename: string;
  hash: string;
  sizeBytes: number;
  status: 'QUARANTINED' | 'VALIDATED';
  totalRecordsParsed: number;
  totalEntriesParsed: number;
  totalDebitsMinor: number;
  totalCreditsMinor: number;
  policyVersion: string;
  contractId?: string;
  notCheckedRuleIds?: string[];
  quarantineReasons?: string[];
  findings: unknown[];
  incidentId?: number;
}

export const ingestRaw = (
  filename: string,
  content: string,
): Promise<ApiResult<IngestionResult>> =>
  request<IngestionResult>('/files/ingest-raw', {
    method: 'POST',
    body: { filename, content },
  });

export { query };

// ---------------------------------------------------------------------------
// SentinelFlow Lens — governed advisory analytics
// ---------------------------------------------------------------------------

export const getLensDatasets = (signal?: AbortSignal): Promise<ApiResult<LensDataset[]>> =>
  request<LensDataset[]>('/lens/datasets', { signal });

export const runLensQuery = (
  intent: LensQueryIntent,
  signal?: AbortSignal,
): Promise<ApiResult<LensQueryResult>> =>
  request<LensQueryResult>('/lens/query', { method: 'POST', body: intent, signal });

export const createLensInvestigation = (
  title: string,
): Promise<ApiResult<LensInvestigation>> =>
  request<LensInvestigation>('/lens/investigations', { method: 'POST', body: { title } });

export const getLensInvestigation = (
  id: string,
  signal?: AbortSignal,
): Promise<ApiResult<LensInvestigation>> =>
  request<LensInvestigation>(`/lens/investigations/${encodeURIComponent(id)}`, { signal });

export const addLensInvestigationNode = (
  investigationId: string,
  question: string,
  queryIntent: LensQueryIntent,
  parentNodeId?: string,
): Promise<ApiResult<LensInvestigationNode>> =>
  request<LensInvestigationNode>(
    `/lens/investigations/${encodeURIComponent(investigationId)}/nodes`,
    { method: 'POST', body: { parent_node_id: parentNodeId ?? '', question, query_intent: queryIntent } },
  );
