import assert from 'node:assert/strict';
import { test } from 'node:test';

import { askAll, pickBlock, restorePicks } from '../src/lib/chat.ts';
import type { ConnectionType } from '../src/lib/api.ts';

const online: { key: string; type: ConnectionType }[] = [
  { key: '1|llama', type: 'local' },
  { key: '1|qwen', type: 'local' },
  { key: '1|mistral', type: 'local' },
  { key: '2|claude', type: 'subscription' },
  { key: '3|gpt', type: 'api' },
  { key: '3|gemini', type: 'api' },
];

test('a model joins while there is room', () => {
  assert.equal(pickBlock([], 'local'), '');
  assert.equal(pickBlock(['local', 'subscription', 'api'], 'subscription'), '');
});

test('four models at most', () => {
  assert.notEqual(pickBlock(['local', 'api', 'subscription', 'subscription'], 'api'), '');
});

test('two local models at most, cloud ones still join', () => {
  assert.notEqual(pickBlock(['local', 'local'], 'local'), '');
  assert.equal(pickBlock(['local', 'local'], 'subscription'), '');
});

test('the saved picks come back in their order', () => {
  assert.deepEqual(restorePicks(['2|claude', '1|llama'], online), ['2|claude', '1|llama']);
});

test('a saved model gone offline is dropped', () => {
  assert.deepEqual(restorePicks(['9|gone', '3|gpt'], online), ['3|gpt']);
});

test('saved picks beyond the caps are dropped', () => {
  assert.deepEqual(restorePicks(['1|llama', '1|qwen', '1|mistral', '2|claude'], online), ['1|llama', '1|qwen', '2|claude']);
  assert.deepEqual(restorePicks(['1|llama', '2|claude', '3|gpt', '3|gemini', '1|qwen'], online), ['1|llama', '2|claude', '3|gpt', '3|gemini']);
});

test('a malformed saved value picks nothing', () => {
  assert.deepEqual(restorePicks(null, online), []);
  assert.deepEqual(restorePicks({ key: '1|llama' }, online), []);
  assert.deepEqual(restorePicks(['1|llama', '1|llama', 7], online), ['1|llama']);
});

test('local models answer one after another, cloud ones alongside them', async () => {
  const log: string[] = [];
  await askAll(['local-1', 'cloud-1', 'local-2', 'cloud-2'], (model) => model.startsWith('local'), async (model) => {
    log.push(`start ${model}`);
    await new Promise((resolve) => setTimeout(resolve, 0));
    log.push(`end ${model}`);
  });
  assert.deepEqual(log.slice(0, 3), ['start local-1', 'start cloud-1', 'start cloud-2']);
  assert.ok(log.indexOf('start local-2') > log.indexOf('end local-1'));
  assert.equal(log.length, 8);
});
