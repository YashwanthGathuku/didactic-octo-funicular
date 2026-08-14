import { DomainEvent } from '../types/financial';

/**
 * Fast SHA-256 string hash calculation (using Web Crypto API with fallback)
 */
export async function calculateSha256(text: string): Promise<string> {
  if (typeof crypto !== 'undefined' && crypto.subtle) {
    const msgUint8 = new TextEncoder().encode(text);
    const hashBuffer = await crypto.subtle.digest('SHA-256', msgUint8);
    const hashArray = Array.from(new Uint8Array(hashBuffer));
    return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
  }
  
  // Deterministic fallback hash for non-crypto environments
  let h1 = 0xdeadbeef, h2 = 0x41c6ce57;
  for (let i = 0; i < text.length; i++) {
    const ch = text.charCodeAt(i);
    h1 = Math.imul(h1 ^ ch, 2654435761);
    h2 = Math.imul(h2 ^ ch, 1597334677);
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507);
  h1 ^= Math.imul(h2 ^ (h2 >>> 13), 3266489909);
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507);
  h2 ^= Math.imul(h1 ^ (h1 >>> 13), 3266489909);
  return (4294967296 * (2097151 & h2) + (h1 >>> 0)).toString(16).padStart(64, '0');
}

export class TamperEvidentEventStore {
  private events: DomainEvent[] = [];
  private latestHash = '0000000000000000000000000000000000000000000000000000000000000000';

  public async appendEvent(
    tenantId: string,
    aggregateId: string,
    aggregateType: DomainEvent['aggregateType'],
    eventType: string,
    actor: string,
    payload: Record<string, unknown>,
    correlationId: string,
    causationId?: string
  ): Promise<DomainEvent> {
    const timestampUtc = new Date().toISOString();
    const previousHash = this.latestHash;

    const eventContentForHash = JSON.stringify({
      previousHash,
      tenantId,
      aggregateId,
      aggregateType,
      eventType,
      timestampUtc,
      actor,
      correlationId,
      payload
    });

    const currentHash = await calculateSha256(eventContentForHash);

    const event: DomainEvent = {
      id: `EVT-${Date.now()}-${Math.random().toString(36).substring(2, 7)}`,
      tenantId,
      aggregateId,
      aggregateType,
      eventType,
      timestampUtc,
      actor,
      correlationId,
      causationId,
      payload,
      previousHash,
      currentHash
    };

    this.events.push(event);
    this.latestHash = currentHash;
    return event;
  }

  public getEvents(): DomainEvent[] {
    return [...this.events];
  }

  public async verifyIntegrity(): Promise<{
    isValid: boolean;
    totalEvents: number;
    tamperedEventIndex?: number;
    error?: string;
  }> {
    let expectedPreviousHash = '0000000000000000000000000000000000000000000000000000000000000000';

    for (let i = 0; i < this.events.length; i++) {
      const event = this.events[i];
      if (event.previousHash !== expectedPreviousHash) {
        return {
          isValid: false,
          totalEvents: this.events.length,
          tamperedEventIndex: i,
          error: `Broken chain link at index ${i}: expected previousHash '${expectedPreviousHash}', found '${event.previousHash}'.`
        };
      }

      const contentForHash = JSON.stringify({
        previousHash: event.previousHash,
        tenantId: event.tenantId,
        aggregateId: event.aggregateId,
        aggregateType: event.aggregateType,
        eventType: event.eventType,
        timestampUtc: event.timestampUtc,
        actor: event.actor,
        correlationId: event.correlationId,
        payload: event.payload
      });

      const recalculate = await calculateSha256(contentForHash);
      if (recalculate !== event.currentHash) {
        return {
          isValid: false,
          totalEvents: this.events.length,
          tamperedEventIndex: i,
          error: `Payload tamper detected at event index ${i} ('${event.id}'): calculated hash '${recalculate}' does not match stored hash '${event.currentHash}'.`
        };
      }

      expectedPreviousHash = event.currentHash;
    }

    return {
      isValid: true,
      totalEvents: this.events.length
    };
  }
}
