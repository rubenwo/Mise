import { describe, expect, it } from 'vitest';
import { parseSSEChunk } from './useGeneration';

describe('parseSSEChunk', () => {
  it('parses complete data lines and keeps the partial remainder', () => {
    const { events, rest } = parseSSEChunk(
      'data: {"type":"status","message":"hi"}\n\ndata: {"type":"rec',
    );
    expect(events).toEqual([{ type: 'status', message: 'hi' }]);
    expect(rest).toBe('data: {"type":"rec');
  });

  it('completes an event split across chunks', () => {
    const first = parseSSEChunk('data: {"type":"rec');
    const second = parseSSEChunk(first.rest + 'ipe","index":2}\n\n');
    expect(second.events).toEqual([{ type: 'recipe', index: 2 }]);
    expect(second.rest).toBe('');
  });

  it('skips non-data lines and malformed JSON', () => {
    const { events } = parseSSEChunk(': keepalive\ndata: {not json}\ndata: {"type":"error"}\n');
    expect(events).toEqual([{ type: 'error' }]);
  });
});
