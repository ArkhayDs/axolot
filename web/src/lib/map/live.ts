// What the running benchmark touches on the map. Type imports only, so the node test runner loads it as is.

import type { Run } from '../api';
import type { MapContent } from './content';
import type { NodeKind } from './graph';

/**
 * Same ids as graph's nodeId, which this module cannot import at runtime
 * @param {NodeKind} kind - node type
 * @param {string} key - identity within the type
 * @returns {string} - node id
 */
function idOf(kind: NodeKind, key: string): string {
  return `${kind}:${key}`;
}

/**
 * Nodes the running benchmark touches: its models, the scenarios it plays as a whole, their themes, the machine and itself
 * @param {MapContent} content - what the map holds
 * @param {{ campaign: string; model: string; caseId: string; runs: Pick<Run, 'label' | 'case_id'>[] }} live - running benchmark, runs of the shown type
 * @returns {Set<string>} - node ids
 */
export function liveNodes(content: MapContent, live: { campaign: string; model: string; caseId: string; runs: Pick<Run, 'label' | 'case_id'>[] }): Set<string> {
  const models = [live.model, ...live.runs.map((run) => run.label)].filter(Boolean);
  const cases = new Set([live.caseId, ...live.runs.map((run) => run.case_id)]);
  // shortcut: every machine counts as the running one, the live state does not carry it; fine while results come from one machine.
  const ids = new Set([idOf('benchmark', live.campaign), ...content.machines.map((machine) => idOf('machine', machine)), ...models.map((model) => idOf('model', model))]);
  for (const scenario of content.scenarios.filter((item) => cases.has(item.id))) {
    ids.add(idOf('theme', scenario.category));
    ids.add(idOf('scenario', scenario.id));
  }
  for (const question of content.questions.filter((item) => cases.has(item.caseId))) ids.add(idOf('question', question.key));
  return ids;
}
