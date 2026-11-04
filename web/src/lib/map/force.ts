// Bounded force layout: repulsion, springs, centering and boxes that keep hub labels clear. Seeds come from the caller, so a layout is reproducible.

export interface Body {
  x: number;
  y: number;
  vx: number;
  vy: number;
  // Where a body returns if a step ever yields a non-finite position.
  seedX: number;
  seedY: number;
  radius: number;
  charge: number;
  // Label to the right of the shape, zero for the small nodes whose labels come and go with the zoom.
  labelWidth: number;
  labelHeight: number;
  pinned: boolean;
}

export interface Spring {
  a: Body;
  b: Body;
  rest: number;
  strength: number;
}

const REPULSION = 320;
const NEAREST_SQUARED = 64;
const GRAVITY = 0.005;
const DAMPING = 0.7;
const MAX_SPEED = 40;
const MAX_STEPS = 320;
const MIN_STEPS = 60;
const ENERGY_FLOOR = 0.01;
// Pair visits allowed for one layout: past it the layout gets coarser, the tab never stalls.
const PAIR_BUDGET = 4e7;
const SETTLE_ROUNDS = 40;

/**
 * Separates overlapping boxes along their shallowest axis
 * @param {Body[]} bodies - nodes
 * @returns {boolean} - whether anything still overlapped
 */
function separate(bodies: Body[]): boolean {
  let moved = false;
  for (let i = 0; i < bodies.length; i += 1) {
    const a = bodies[i];
    const aw = a.radius + a.labelWidth / 2 + 2;
    const ah = Math.max(a.radius, a.labelHeight / 2) + 2;
    for (let j = i + 1; j < bodies.length; j += 1) {
      const b = bodies[j];
      const dx = b.x + b.labelWidth / 2 - a.x - a.labelWidth / 2;
      const dy = b.y - a.y;
      const overlapX = aw + b.radius + b.labelWidth / 2 + 2 - Math.abs(dx);
      if (overlapX <= 0) continue;
      const overlapY = ah + Math.max(b.radius, b.labelHeight / 2) + 2 - Math.abs(dy);
      if (overlapY <= 0) continue;
      moved = true;
      const alongX = overlapX < overlapY;
      const push = (alongX ? overlapX : overlapY) / 2;
      const sign = (alongX ? dx : dy) >= 0 ? 1 : -1;
      const shareA = a.pinned ? 0 : b.pinned ? 2 : 1;
      const shareB = b.pinned ? 0 : a.pinned ? 2 : 1;
      if (alongX) {
        a.x -= sign * push * shareA;
        b.x += sign * push * shareB;
      } else {
        a.y -= sign * push * shareA;
        b.y += sign * push * shareB;
      }
    }
  }
  return moved;
}

/**
 * Advances the simulation one step
 * @param {Body[]} bodies - nodes
 * @param {Spring[]} springs - links
 * @param {number} alpha - remaining energy in 0-1
 * @param {number} aspect - free area width over height, the centering pull follows it
 * @returns {number} - mean squared speed, what the caller stops on
 */
export function tick(bodies: Body[], springs: Spring[], alpha: number, aspect: number): number {
  for (let i = 0; i < bodies.length; i += 1) {
    const a = bodies[i];
    for (let j = i + 1; j < bodies.length; j += 1) {
      const b = bodies[j];
      const dx = b.x - a.x || 0.1;
      const dy = b.y - a.y || 0.1;
      const squared = Math.max(dx * dx + dy * dy, NEAREST_SQUARED);
      const force = (REPULSION * a.charge * b.charge * alpha) / squared;
      const distance = Math.sqrt(squared);
      a.vx -= (dx / distance) * force;
      a.vy -= (dy / distance) * force;
      b.vx += (dx / distance) * force;
      b.vy += (dy / distance) * force;
    }
  }
  for (const { a, b, rest, strength } of springs) {
    const dx = b.x - a.x;
    const dy = b.y - a.y;
    const distance = Math.hypot(dx, dy) || 1;
    const force = (distance - rest) * strength * alpha;
    a.vx += (dx / distance) * force;
    a.vy += (dy / distance) * force;
    b.vx -= (dx / distance) * force;
    b.vy -= (dy / distance) * force;
  }
  let energy = 0;
  for (const body of bodies) {
    body.vx -= (body.x * GRAVITY * alpha) / Math.max(aspect, 1);
    body.vy -= body.y * GRAVITY * alpha * Math.max(aspect, 1);
    if (body.pinned) {
      body.vx = 0;
      body.vy = 0;
      continue;
    }
    body.vx = Math.max(-MAX_SPEED, Math.min(MAX_SPEED, body.vx * DAMPING));
    body.vy = Math.max(-MAX_SPEED, Math.min(MAX_SPEED, body.vy * DAMPING));
    body.x += body.vx;
    body.y += body.vy;
    if (!Number.isFinite(body.x + body.y + body.vx + body.vy)) {
      body.x = body.seedX;
      body.y = body.seedY;
      body.vx = 0;
      body.vy = 0;
    }
    energy += body.vx * body.vx + body.vy * body.vy;
  }
  separate(bodies);
  return energy / Math.max(bodies.length, 1);
}

/**
 * Computes the whole layout at once: capped steps, stopped early once nothing moves
 * @param {Body[]} bodies - nodes
 * @param {Spring[]} springs - links
 * @param {number} aspect - free area width over height
 * @returns {number} - steps played
 */
export function solve(bodies: Body[], springs: Spring[], aspect: number): number {
  const pairs = Math.max((bodies.length * (bodies.length - 1)) / 2, 1);
  const steps = Math.min(MAX_STEPS, Math.floor(PAIR_BUDGET / pairs));
  let played = 0;
  for (; played < steps; played += 1) {
    const energy = tick(bodies, springs, 1 - played / steps, aspect);
    if (played > MIN_STEPS && energy < ENERGY_FLOOR) break;
  }
  for (const body of bodies) {
    body.vx = 0;
    body.vy = 0;
  }
  settle(bodies);
  return played;
}

/**
 * Finishes the collisions the forces left open
 * @param {Body[]} bodies - nodes
 */
export function settle(bodies: Body[]): void {
  const pairs = Math.max((bodies.length * (bodies.length - 1)) / 2, 1);
  const rounds = Math.min(SETTLE_ROUNDS, Math.floor(PAIR_BUDGET / 4 / pairs));
  for (let round = 0; round < rounds && separate(bodies); round += 1);
}
