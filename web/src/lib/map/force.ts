// Hand-written force layout: repulsion, springs, centering, and boxes that keep labels clear of each other.

export interface Body {
  x: number;
  y: number;
  vx: number;
  vy: number;
  radius: number;
  // Label size below the shape, so a node never lands on another node's text.
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

const BOX_MARGIN = 10;
const DAMPING = 0.72;
const STEPS = 340;

/**
 * @param {Body} body - node
 * @returns {{ cx: number; cy: number; hw: number; hh: number }} - box around shape and label
 */
function boxOf(body: Body): { cx: number; cy: number; hw: number; hh: number } {
  return {
    cx: body.x,
    cy: body.y + body.labelHeight / 2,
    hw: Math.max(body.radius, body.labelWidth / 2) + BOX_MARGIN,
    hh: body.radius + body.labelHeight / 2 + BOX_MARGIN,
  };
}

/**
 * Separates overlapping boxes along their shallowest axis
 * @param {Body[]} bodies - nodes
 * @returns {boolean} - whether anything still overlapped
 */
function separate(bodies: Body[]): boolean {
  let moved = false;
  for (let i = 0; i < bodies.length; i += 1) {
    for (let j = i + 1; j < bodies.length; j += 1) {
      const a = boxOf(bodies[i]);
      const b = boxOf(bodies[j]);
      const dx = b.cx - a.cx;
      const dy = b.cy - a.cy;
      const overlapX = a.hw + b.hw - Math.abs(dx);
      const overlapY = a.hh + b.hh - Math.abs(dy);
      if (overlapX <= 0 || overlapY <= 0) continue;
      moved = true;
      const alongX = overlapX < overlapY;
      const push = (alongX ? overlapX : overlapY) / 2;
      const sign = (alongX ? dx : dy) >= 0 ? 1 : -1;
      const shareA = bodies[i].pinned ? 0 : bodies[j].pinned ? 2 : 1;
      const shareB = bodies[j].pinned ? 0 : bodies[i].pinned ? 2 : 1;
      if (alongX) {
        bodies[i].x -= sign * push * shareA;
        bodies[j].x += sign * push * shareB;
      } else {
        bodies[i].y -= sign * push * shareA;
        bodies[j].y += sign * push * shareB;
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
 * @param {number} scale - viewport size relative to the reference one
 * @param {number} aspect - viewport width over height, the centering pull follows it
 */
export function tick(bodies: Body[], springs: Spring[], alpha: number, scale: number, aspect: number): void {
  const repulsion = 150000 * scale * scale;
  for (let i = 0; i < bodies.length; i += 1) {
    for (let j = i + 1; j < bodies.length; j += 1) {
      const a = bodies[i];
      const b = bodies[j];
      const dx = b.x - a.x || 0.1;
      const dy = b.y - a.y || 0.1;
      const squared = Math.max(dx * dx + dy * dy, 400);
      const force = (repulsion / squared) * alpha;
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
    const force = (distance - rest * scale) * strength * alpha;
    a.vx += (dx / distance) * force;
    a.vy += (dy / distance) * force;
    b.vx -= (dx / distance) * force;
    b.vy -= (dy / distance) * force;
  }
  for (const body of bodies) {
    body.vx -= (body.x * 0.018 * alpha) / Math.max(aspect, 1);
    body.vy -= body.y * 0.018 * alpha * Math.max(aspect, 1);
    if (body.pinned) {
      body.vx = 0;
      body.vy = 0;
      continue;
    }
    body.vx *= DAMPING;
    body.vy *= DAMPING;
    body.x += body.vx;
    body.y += body.vy;
  }
  separate(bodies);
}

/**
 * Computes the whole layout at once, nothing moves on screen at rest
 * @param {Body[]} bodies - nodes
 * @param {Spring[]} springs - links
 * @param {number} scale - viewport size relative to the reference one
 * @param {number} aspect - viewport width over height
 */
export function solve(bodies: Body[], springs: Spring[], scale: number, aspect: number): void {
  for (let step = 0; step < STEPS; step += 1) tick(bodies, springs, 1 - step / STEPS, scale, aspect);
  for (const body of bodies) {
    body.vx = 0;
    body.vy = 0;
  }
  settle(bodies);
}

/**
 * Finishes the collisions the forces left open
 * @param {Body[]} bodies - nodes
 */
export function settle(bodies: Body[]): void {
  for (let round = 0; round < 60 && separate(bodies); round += 1);
}
