import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { MapContent } from '../src/lib/map/content.ts';
import { liveNodes } from '../src/lib/map/live.ts';

const content = {
  machines: ['Mac'],
  scenarios: [
    { id: 'a', category: 'rag', items: [] },
    { id: 'b', category: 'code', items: [] },
    { id: 'c', category: 'code', items: [] },
  ],
  questions: [
    { key: 'a/1', caseId: 'a' },
    { key: 'a/2', caseId: 'a' },
    { key: 'b/1', caseId: 'b' },
    { key: 'c/1', caseId: 'c' },
  ],
} as unknown as MapContent;

test('keeps the played and the current scenarios whole, their themes, models, machine and benchmark', () => {
  const ids = liveNodes(content, { campaign: 'x', model: 'qwen', caseId: 'b', runs: [{ label: 'llama', case_id: 'a' }] });
  assert.deepEqual(
    [...ids].sort(),
    ['benchmark:x', 'machine:Mac', 'model:llama', 'model:qwen', 'question:a/1', 'question:a/2', 'question:b/1', 'scenario:a', 'scenario:b', 'theme:code', 'theme:rag'],
  );
});

test('a benchmark not started yet keeps only itself and the machine', () => {
  assert.deepEqual([...liveNodes(content, { campaign: 'x', model: '', caseId: '', runs: [] })].sort(), ['benchmark:x', 'machine:Mac']);
});
