import React, { useEffect, useState } from 'react';
import { AlertTriangle, WifiOff, CheckCircle2 } from 'lucide-react';
import { SentinelApi } from '../services/api';

/**
 * DemoDataBanner states two facts an operator cannot otherwise see.
 *
 * 1. The expectation board, partners and contracts on this screen are a local
 *    synthetic corpus (src/mockData/syntheticCorpus.ts). They are NOT read from
 *    the gateway. Before Prompt 01 nothing on screen said so, and synthetic
 *    state was visually identical to real state.
 *
 * 2. Whether the gateway is actually reachable. Previously a backend outage was
 *    invisible: the API client caught every error, logged "using local mock
 *    state" to a console nobody watches, and returned an empty array, so an
 *    outage rendered as a healthy, empty board.
 *
 * Prompt 12 rebuilds these screens on authenticated server data, at which point
 * the demo half of this banner is deleted and the connectivity half becomes a
 * real per-query degraded state.
 */

type GatewayState = 'checking' | 'reachable' | 'unreachable' | 'unauthorized';

export const DemoDataBanner: React.FC = () => {
  const [gateway, setGateway] = useState<GatewayState>('checking');
  const [detail, setDetail] = useState<string>('');

  useEffect(() => {
    let cancelled = false;

    const poll = async () => {
      const res = await SentinelApi.checkHealth();
      if (cancelled) return;
      if (res.state === 'ok') {
        setGateway('reachable');
        setDetail('');
      } else if (res.state === 'unauthorized') {
        setGateway('unauthorized');
        setDetail(res.error);
      } else {
        setGateway('unreachable');
        setDetail(res.error);
      }
    };

    poll();
    const id = setInterval(poll, 15000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  const gatewayLine = () => {
    switch (gateway) {
      case 'checking':
        return { icon: <WifiOff size={14} />, text: 'Checking gateway…', color: 'var(--text-muted)' };
      case 'reachable':
        return {
          icon: <CheckCircle2 size={14} />,
          text: 'Gateway reachable. Uploads and triage use live server validation.',
          color: 'var(--accent-emerald)',
        };
      case 'unauthorized':
        return {
          icon: <WifiOff size={14} />,
          text: `Gateway reachable but rejected this client: ${detail}`,
          color: 'var(--accent-amber)',
        };
      case 'unreachable':
        return {
          icon: <WifiOff size={14} />,
          text: `Gateway UNAVAILABLE — ${detail} Nothing on this screen reflects live server state.`,
          color: 'var(--accent-red, #ef4444)',
        };
    }
  };

  const g = gatewayLine();

  return (
    <div
      role="status"
      aria-live="polite"
      className="glass-panel"
      style={{
        padding: '12px 16px',
        border: '1px solid var(--accent-amber)',
        background: 'rgba(245, 158, 11, 0.08)',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        <AlertTriangle size={16} color="var(--accent-amber)" />
        <strong style={{ fontSize: '0.8125rem', letterSpacing: '0.04em', color: 'var(--accent-amber)' }}>
          DEMO DATA
        </strong>
        <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>
          Expectations, partners and contracts shown below come from a local synthetic corpus, not
          from the gateway. Do not read them as operational state.
        </span>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: g.color, fontSize: '0.75rem' }}>
        {g.icon}
        <span>{g.text}</span>
      </div>
    </div>
  );
};
