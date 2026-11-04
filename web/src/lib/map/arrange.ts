// Rearrangement under a selection: distance says the score. Interpolates targets over a bounded time, the simulation stays asleep.

import type { GraphNode } from './graph';

export interface Move {
  node: GraphNode;
  fromX: number;
  fromY: number;
  toX: number;
  toY: number;
}

export interface Selection {
  model: string;
  category: string;
  themeScore: (theme: string) => number | null;
  modelScore: (model: string) => number | null;
}

const NEAREST_ISLAND = 0.7;
const NEAREST_MODEL = 90;
const FARTHEST_MODEL = 330;
const FAN_ANGLE = 0.5;

/**
 * Never past the default ring, so the framing holds whatever the scores
 * @param {number | null} score - model's score on the theme, null when never played
 * @returns {number} - island distance relative to the default one: the better, the closer
 */
export function islandScale(score: number | null): number {
  return score === null ? 1 : 1 - ((1 - NEAREST_ISLAND) * Math.max(0, Math.min(score, 100))) / 100;
}

/**
 * @param {number | null} score - model's score on the selected theme, null when never played
 * @returns {number} - distance from the theme: the better, the closer
 */
export function modelDistance(score: number | null): number {
  return FARTHEST_MODEL - ((FARTHEST_MODEL - NEAREST_MODEL) * Math.max(0, Math.min(score ?? 0, 100))) / 100;
}

/**
 * Plans where everything goes: islands move as a block toward the selected model's center, or models fan out from the selected theme; no selection means home
 * @param {GraphNode[]} nodes - nodes
 * @param {Selection} selection - what is selected, and its scores
 * @returns {Move[]} - moves to play, from where nodes are now
 */
export function planMoves(nodes: GraphNode[], selection: Selection): Move[] {
  const moves: Move[] = [];
  const go = (node: GraphNode, toX: number, toY: number): void => {
    if (Math.abs(toX - node.x) + Math.abs(toY - node.y) > 0.01) moves.push({ node, fromX: node.x, fromY: node.y, toX, toY });
  };
  const hubs = new Map(nodes.filter((node) => node.kind === 'theme').map((hub) => [hub.key, hub]));
  const scaleOf = (theme: string): number => (selection.model ? islandScale(selection.themeScore(theme)) : 1);
  for (const node of nodes) {
    const hub = hubs.get(node.community);
    if (!hub) continue;
    const scale = scaleOf(hub.key);
    node.anchorX = hub.seedX * scale;
    node.anchorY = hub.seedY * scale;
    go(node, node.x + hub.homeX * scale - hub.x, node.y + hub.homeY * scale - hub.y);
  }
  const center = !selection.model && selection.category ? hubs.get(selection.category) : undefined;
  const models = nodes.filter((node) => node.kind === 'model');
  models.forEach((node, position) => {
    // The selected model takes the center the islands gather around.
    let [toX, toY] = [node.key === selection.model ? 0 : node.homeX, node.homeY];
    if (center) {
      const angle = Math.atan2(-center.homeY, -center.homeX) + (position - (models.length - 1) / 2) * FAN_ANGLE;
      const distance = modelDistance(selection.modelScore(node.key));
      toX = center.homeX + Math.cos(angle) * distance;
      toY = center.homeY + Math.sin(angle) * distance;
    }
    node.anchorX = toX;
    node.anchorY = toY;
    go(node, toX, toY);
  });
  return moves;
}

/**
 * @param {Move[]} moves - planned moves
 * @param {number} progress - elapsed share of the animation, 0-1
 */
export function applyMoves(moves: Move[], progress: number): void {
  const eased = 1 - (1 - progress) ** 3;
  for (const { node, fromX, fromY, toX, toY } of moves) {
    node.x = fromX + (toX - fromX) * eased;
    node.y = fromY + (toY - fromY) * eased;
  }
}
