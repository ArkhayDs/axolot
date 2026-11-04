import assert from 'node:assert/strict';
import { test } from 'node:test';

import { solve, tick } from '../src/lib/map/force.ts';
import type { Body, Spring } from '../src/lib/map/force.ts';
import { islandScale, modelDistance } from '../src/lib/map/arrange.ts';
import { stateOfItem, stateOfScore } from '../src/lib/map/results.ts';

/** @returns {Body} - node at the given place */
function body(x: number, y: number, charge = 1): Body {
  return { x, y, vx: 0, vy: 0, seedX: x, seedY: y, anchorX: 0, anchorY: 0, pull: 0.01, radius: 3, charge, labelWidth: 0, labelHeight: 0, pinned: false };
}

/** @returns {{ bodies: Body[]; springs: Spring[] }} - a graph of the given size, chained two by two */
function chain(size: number): { bodies: Body[]; springs: Spring[] } {
  const bodies = Array.from({ length: size }, (_, position) => body(Math.cos(position) * position, Math.sin(position) * position));
  const springs = bodies.slice(1).map((b, position) => ({ a: bodies[position], b, rest: 20, strength: 0.1 }));
  return { bodies, springs };
}

test('same seeds give the same layout', () => {
  const first = chain(120);
  const second = chain(120);
  solve(first.bodies, first.springs);
  solve(second.bodies, second.springs);
  assert.deepEqual(first.bodies.map((node) => [node.x, node.y]), second.bodies.map((node) => [node.x, node.y]));
});

test('survives no node, one node and nodes on the same spot', () => {
  solve([], []);
  const alone = [body(0, 0)];
  solve(alone, []);
  assert.ok(Number.isFinite(alone[0].x));
  const stacked = [body(5, 5), body(5, 5), body(5, 5)];
  solve(stacked, []);
  assert.ok(stacked.every((node) => Number.isFinite(node.x) && Number.isFinite(node.y)));
});

test('a poisoned position returns to its seed instead of spreading', () => {
  const { bodies, springs } = chain(10);
  bodies[3].x = NaN;
  tick(bodies, springs, 1);
  tick(bodies, springs, 1);
  solve(bodies, springs);
  assert.ok(bodies.every((node) => Number.isFinite(node.x) && Number.isFinite(node.y)));
});

test('a huge graph stays within the pair budget', () => {
  const { bodies, springs } = chain(4000);
  const start = performance.now();
  const steps = solve(bodies, springs);
  assert.ok(steps <= 5, `steps=${steps}`);
  assert.ok(performance.now() - start < 3000);
});

test('anchors hold two islands apart', () => {
  const left = Array.from({ length: 10 }, (_, position) => ({ ...body(position, position), anchorX: -300, pull: 0.1 }));
  const right = Array.from({ length: 10 }, (_, position) => ({ ...body(position, -position), anchorX: 300, pull: 0.1 }));
  solve([...left, ...right], []);
  assert.ok(left.every((node) => node.x < -150), 'left island stays left');
  assert.ok(right.every((node) => node.x > 150), 'right island stays right');
});

test('stops once nothing moves, within the step cap', () => {
  const { bodies, springs } = chain(120);
  assert.ok(solve(bodies, springs) <= 320);
});

test('the better the score, the closer, up to the default ring', () => {
  assert.ok(islandScale(100) < islandScale(80));
  assert.ok(islandScale(80) < islandScale(0));
  assert.equal(islandScale(0), 1);
  assert.equal(islandScale(null), 1);
  assert.ok(modelDistance(100) < modelDistance(50));
  assert.ok(modelDistance(50) < modelDistance(0));
});

test('reads a result like the answer matrix', () => {
  assert.equal(stateOfScore(100, false), 'passed');
  assert.equal(stateOfScore(99.99, false), 'passed');
  assert.equal(stateOfScore(50, false), 'partial');
  assert.equal(stateOfScore(0, false), 'failed');
  assert.equal(stateOfScore(80, true), 'failed');
  assert.equal(stateOfItem(undefined), 'none');
  assert.equal(stateOfItem({ machine: '', model: 'm', case_id: 'c', item_id: 'i', runs: 2, avg_score: 40, p50_total_ms: 1, errors: 1 }), 'partial');
  assert.equal(stateOfItem({ machine: '', model: 'm', case_id: 'c', item_id: 'i', runs: 2, avg_score: 0, p50_total_ms: 1, errors: 2 }), 'failed');
});
