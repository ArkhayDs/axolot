import assert from 'node:assert/strict';
import { test } from 'node:test';

import { connectionTypeOf, modelParts, shortModelName } from '../src/lib/format.ts';

test('classifies a connection like the server', () => {
  assert.equal(connectionTypeOf('ollama'), 'local');
  assert.equal(connectionTypeOf('openai'), 'api');
  assert.equal(connectionTypeOf('bridge'), 'subscription');
});

test('splits a label into family, tag and connection', () => {
  assert.deepEqual(modelParts('deepseek/deepseek-r1:free (API · OpenRouter)'), { family: 'deepseek/deepseek-r1', tag: ':free', via: ' (API · OpenRouter)' });
  assert.deepEqual(modelParts('qwen3:8b'), { family: 'qwen3', tag: ':8b', via: '' });
  assert.deepEqual(modelParts('claude-haiku-5-5 (abo)'), { family: 'claude-haiku-5-5', tag: '', via: ' (abo)' });
});

test('a short name keeps the connection', () => {
  assert.equal(shortModelName('gpt-oss:120b (API · OpenAI)'), 'gpt-oss (API · OpenAI)');
});
