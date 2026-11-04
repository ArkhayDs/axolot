import assert from 'node:assert/strict';
import { test } from 'node:test';

import { percentile, qualityRows, sentencesOf, wordsOf } from '../src/lib/qualimetry.ts';
import type { Run } from '../src/lib/api.ts';

/** @returns {Run} - answer with the given overrides */
function run(overrides: Partial<Run>): Run {
  return { id: '', campaign_id: '', case_id: 'c', item_id: 'i', category: 'tri', provider_name: 'p', model: 'm', output: '', error: '', ttft_ms: null, total_ms: 1, load_ms: null, prompt_tokens: null, output_tokens: null, tok_per_sec: null, score: 0, checks: [], retrieved: null, retrieval_hit: null, ...overrides };
}

test('counts French words and keeps decimals in one sentence', () => {
  assert.deepEqual(wordsOf("L'avoir est-il dû ? 3,5 %"), ["l'avoir", 'est-il', 'dû', '3', '5']);
  assert.equal(sentencesOf('Le prix est 3.5 euros. Vrai ? Oui\nFin').length, 4);
});

test('interpolates percentiles like the database', () => {
  assert.equal(percentile([1, 2, 3, 4], 0.5), 2.5);
  assert.equal(percentile([], 0.5), null);
});

test('computes indicators only where they apply', () => {
  const rows = qualityRows(
    [
      run({ output: 'un deux deux. trois' }),
      run({ output: '' }),
      run({ output: '{"a":1}', checks: [{ name: 'json_valid', passed: true }] }),
      run({ output: '{"a":', checks: [{ name: 'json_valid', passed: false }], output_tokens: 100 }),
    ],
    100,
  );
  assert.equal(rows.length, 2);
  const [row] = rows;
  assert.equal(row.answers, 4);
  assert.equal(row.jsonRate, 0.5);
  assert.equal(row.citationRate, null);
  assert.equal(row.blankRate, 0.5);
  assert.equal(row.sentenceWords, 2);
  assert.equal(row.diversity, 0.75);
});
