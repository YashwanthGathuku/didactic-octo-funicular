/**
 * Sentinel Flow - Real-Time API Client
 * Connects the React Operations Cockpit to the Go Gateway & Python AI Tier.
 */

const API_BASE_URL = 'http://localhost:8080/api/v1';

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
  status: 'QUARANTINED' | 'RELEASED';
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
  async checkHealth(): Promise<ApiHealth | null> {
    try {
      const res = await fetch(`${API_BASE_URL}/health`, { method: 'GET' });
      if (!res.ok) return null;
      return await res.json();
    } catch {
      return null;
    }
  },

  async getSlaBoard(): Promise<any[]> {
    try {
      const res = await fetch(`${API_BASE_URL}/sla-board`);
      if (!res.ok) throw new Error('Failed to fetch SLA board');
      return await res.json();
    } catch (e) {
      console.warn('Backend SLA Board unavailable, using local mock state', e);
      return [];
    }
  },

  async getIncidents(): Promise<any[]> {
    try {
      const res = await fetch(`${API_BASE_URL}/incidents`);
      if (!res.ok) throw new Error('Failed to fetch incidents');
      return await res.json();
    } catch (e) {
      console.warn('Backend incidents unavailable, using local mock state', e);
      return [];
    }
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
