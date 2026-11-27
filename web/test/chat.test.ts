import assert from 'node:assert/strict';
import { test } from 'node:test';

import { addEvent, answerText, askAll, awaitsUser, pickBlock, picksToSave, prettyJson, restorePicks, toolSummary } from '../src/lib/chat.ts';
import type { AnswerPart, ToolPart } from '../src/lib/chat.ts';
import { streamChat } from '../src/lib/api.ts';
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

test('a saved model offline for this visit stays stored', () => {
  assert.deepEqual(picksToSave(['1|llama'], ['2|claude', '1|llama'], ['1|llama', '3|gpt']), ['1|llama', '2|claude']);
  assert.deepEqual(picksToSave(['1|llama'], ['3|gpt', 7], ['1|llama', '3|gpt']), ['1|llama']);
  assert.deepEqual(picksToSave(['1|llama'], null, ['1|llama']), ['1|llama']);
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

test('text around tool calls stays in order, each result joins its call', () => {
  const parts: AnswerPart[] = [];
  addEvent(parts, { type: 'delta', text: 'Je ' });
  addEvent(parts, { type: 'delta', text: 'cherche.' });
  addEvent(parts, { type: 'tool_call', name: 'sentinelle__kpis', arguments: '{}' });
  addEvent(parts, { type: 'tool_result', result: '42', error: false, refused: false, ms: 300 });
  addEvent(parts, { type: 'delta', text: 'Réponse : 42' });
  assert.deepEqual(parts, [
    { kind: 'text', text: 'Je cherche.' },
    { kind: 'tool', name: 'sentinelle__kpis', arguments: '{}', result: '42', error: false, ms: 300, approval: null, refused: false, decided: false },
    { kind: 'text', text: 'Réponse : 42' },
  ]);
  assert.equal(answerText(parts), 'Je cherche.\n\nRéponse : 42');
});

test('a tool result without its call fails loudly', () => {
  assert.throws(() => addEvent([], { type: 'tool_result', result: '42', error: false, refused: false, ms: 300 }));
});

test('an approval request joins the call just before it', () => {
  const parts: AnswerPart[] = [];
  addEvent(parts, { type: 'tool_call', name: 'sentinelle__run_sql', arguments: '{}' });
  addEvent(parts, { type: 'approval', id: 'abc' });
  assert.equal(parts[0].kind === 'tool' && parts[0].approval, 'abc');
});

test('a refused call stays marked refused, apart from a failure', () => {
  const parts: AnswerPart[] = [];
  addEvent(parts, { type: 'tool_call', name: 'sentinelle__run_sql', arguments: '{}' });
  addEvent(parts, { type: 'tool_result', result: 'Error: refused', error: true, refused: true, ms: 0 });
  assert.equal(parts[0].kind === 'tool' && parts[0].refused, true);
});

test('a call waits for the user until clicked, answered or its answer stops', () => {
  const parts: AnswerPart[] = [];
  addEvent(parts, { type: 'tool_call', name: 'sentinelle__run_sql', arguments: '{}' });
  const step = parts[0] as ToolPart;
  assert.equal(awaitsUser(step, true), false);
  addEvent(parts, { type: 'approval', id: 'abc' });
  assert.equal(awaitsUser(step, true), true);
  assert.equal(awaitsUser(step, false), false);
  assert.equal(awaitsUser({ ...step, decided: true }, true), false);
  addEvent(parts, { type: 'tool_result', result: '42', error: false, refused: false, ms: 300 });
  assert.equal(awaitsUser(step, true), false);
});

test('an approval request without its call fails loudly', () => {
  assert.throws(() => addEvent([], { type: 'approval', id: 'abc' }));
});

test('the tool summary counts calls and their time, a running call adds none', () => {
  const parts: AnswerPart[] = [];
  assert.deepEqual(toolSummary(parts), { calls: 0, ms: 0 });
  addEvent(parts, { type: 'tool_call', name: 'a', arguments: '{}' });
  addEvent(parts, { type: 'tool_result', result: 'Error: boom', error: true, refused: false, ms: 120 });
  addEvent(parts, { type: 'tool_call', name: 'b', arguments: '{}' });
  assert.deepEqual(toolSummary(parts), { calls: 2, ms: 120 });
});

test('valid arguments are indented, invalid ones shown as the model wrote them', () => {
  assert.equal(prettyJson('{"scope":"mdm"}'), '{\n  "scope": "mdm"\n}');
  assert.equal(prettyJson('{scope:'), '{scope:');
});

test('a failing event handler closes the stream', async () => {
  let closed = false;
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new TextEncoder().encode('{"type":"delta","text":"Bonjour"}\n'));
    },
    cancel() {
      closed = true;
    },
  });
  globalThis.fetch = async (): Promise<Response> => new Response(body);
  const fail = (): void => {
    throw new Error('handler');
  };
  await assert.rejects(streamChat({ provider_id: 1, model: 'llama', messages: [] }, fail, new AbortController().signal), /handler/);
  assert.ok(closed);
});
