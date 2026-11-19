import assert from 'node:assert/strict';
import { test } from 'node:test';

import { pickBlock } from '../src/lib/chat.ts';

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
