/**
 * Sentinel Flow - Real-Time API Client
 * Connects the React Operations Cockpit to the Go Gateway & Python AI Tier.
 */

const API_BASE_URL = 'http://localhost:8080/api/v1';

/**
 * ApiResult makes dependency failure a state the UI must handle, rather than
 * something a catch block can quietly swallow.
 *
 * getSlaBoard() and getIncidents() previously caught every error, logged
 * "using local mock state", and returned []. A backend outage was therefore
 * indistinguishable from "no incidents" -- the screen rendered as healthy and
 * empty. Callers must now branch on `state`.
 *
 * Prompt 12 replaces this with a generated, typed API client that also carries
 * authentication; Prompt 04 supplies the credentials it will send.
 */
export type ApiResult<T> =
  | { state: 'ok'; data: T }
  | { state: 'unavailable'; error: string }
  | { state: 'unauthorized'; error: string };

async function request<T>(url: string, init?: RequestInit): Promise<ApiResult<T>> {
  try {
    const res = await fetch(url, init);
    if (res.status === 401 || res.status === 403) {
      return { state: 'unauthorized', error: `Not authorized (HTTP ${res.status}).` };
    }
    if (!res.ok) {
      return { state: 'unavailable', error: `Gateway returned HTTP ${res.status}.` };
    }
    return { state: 'ok', data: (await res.json()) as T };
  } catch (e) {
    return {
      state: 'unavailable',
      error: e instanceof Error ? e.message : 'Gateway unreachable.',
    };
  }
}

export interface ApiHealth {
  status: string;
  service: string;
  engine: string;
}

export interface ApiLedgerSummary {
  totalEvents: number;
  isChainValid: boolean;
  lastEventHash: string;
  events: {
    id: number;
    eventType: string;
    actor: string;
    payload: Record<string, any>;
    previousHash: string;
    currentHash: string;
    createdAt: string;
  }[];
}

export interface ApiIngestionResult {
  fileId: number;
  filename: string;
  hash: string;
  sizeBytes: number;
  // Terminus of ingestion. RELEASED is NOT produced here: release requires a
  // policy decision and approval, which ingestion does not perform.
  status: 'QUARANTINED' | 'VALIDATED';
  totalRecordsParsed: number;
  totalDebitsUsd: number;
  totalCreditsUsd: number;
  calculatedHash: string;
  expectedHash: string;
  isBalanced: boolean;
  findings: {
    id: number;
    code: string;
    description: string;
    severity: string;
    lineNumber: number;
    rawData: string;
    ruleReference: string;
  }[];
  rawContent?: string;
  incidentId?: number;
}

export interface ApiAnalystResponse {
  summary: string;
  citations: string[];
  proposed_actions: {
    type: string;
    description: string;
  }[];
  confidence: number;
  agent_version: string;
  metrics: {
    durationMs: number;
    inputTokens: number;
    outputTokens: number;
    estimatedCostUsd: number;
  };
}

export const SentinelApi = {
  async checkHealth(): Promise<ApiResult<ApiHealth>> {
    return request<ApiHealth>(`${API_BASE_URL}/health`);
  },

  async getSlaBoard(): Promise<ApiResult<any[]>> {
    return request<any[]>(`${API_BASE_URL}/sla-board`);
  },

  async getIncidents(): Promise<ApiResult<any[]>> {
    return request<any[]>(`${API_BASE_URL}/incidents`);
  },

  async getLedger(): Promise<ApiLedgerSummary | null> {
    try {
      const res = await fetch(`${API_BASE_URL}/ledger`);
      if (!res.ok) return null;
      return await res.json();
    } catch {
      return null;
    }
  },

  async ingestRawNacha(filename: string, content: string): Promise<ApiIngestionResult> {
    const res = await fetch(`${API_BASE_URL}/files/ingest-raw`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ filename, content }),
    });
    if (!res.ok) {
      const errText = await res.text();
      throw new Error(`Ingest failed (${res.status}): ${errText}`);
    }
    return await res.json();
  },

  async triggerTriage(incidentId: string | number): Promise<ApiAnalystResponse> {
    const res = await fetch(`${API_BASE_URL}/incidents/${incidentId}/triage`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
    if (!res.ok) {
      throw new Error(`AI Triage failed with status ${res.status}`);
    }
    return await res.json();
  },

  async approveIncident(incidentId: string | number, actor: string, justification: string): Promise<any> {
    const res = await fetch(`${API_BASE_URL}/incidents/${incidentId}/approve`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ actor, justification }),
    });
    if (!res.ok) {
      throw new Error(`Approval failed with status ${res.status}`);
    }
    return await res.json();
  },

  async triggerChaos(scenario: 'MISSING_FILE' | 'WORKER_CRASH'): Promise<any> {
    const res = await fetch(`${API_BASE_URL}/chaos/trigger`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ scenario }),
    });
    if (!res.ok) {
      throw new Error(`Chaos trigger failed with status ${res.status}`);
    }
    return await res.json();
  },

  async runBenchmark(records: number = 25000): Promise<any> {
    const res = await fetch(`${API_BASE_URL}/benchmark/run?records=${records}`);
    if (!res.ok) throw new Error('Benchmark failed');
    return await res.json();
  },

  async runEvals(): Promise<any> {
    const res = await fetch(`${API_BASE_URL}/evals/run`);
    if (!res.ok) throw new Error('Evals failed');
    return await res.json();
  },

  async getComplianceExport(): Promise<any> {
    const res = await fetch(`${API_BASE_URL}/compliance/export`);
    if (!res.ok) throw new Error('Compliance export failed');
    return await res.json();
  }
};
