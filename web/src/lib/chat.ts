// Chat model picks: each model gets its own answer column.

import type { ConnectionType } from './api';

export const MAX_MODELS = 4;
export const MAX_LOCAL = 2;

/**
 * Why one more model cannot join the picks
 * @param {ConnectionType[]} picked - connection type of each model already picked
 * @param {ConnectionType} type - connection type of the candidate
 * @returns {string} - reason shown to the user, '' when it can join
 */
export function pickBlock(picked: ConnectionType[], type: ConnectionType): string {
  if (picked.length >= MAX_MODELS) return `${MAX_MODELS} modèles au plus`;
  if (type === 'local' && picked.filter((item) => item === 'local').length >= MAX_LOCAL) return `${MAX_LOCAL} modèles locaux au plus`;
  return '';
}

/**
 * Brings back the last picks, keeping those still online and within the caps
 * @param {unknown} saved - parsed stored value, untrusted
 * @param {{ key: string; type: ConnectionType }[]} models - models online
 * @returns {string[]} - keys to pick, in saved order
 */
export function restorePicks(saved: unknown, models: { key: string; type: ConnectionType }[]): string[] {
  if (!Array.isArray(saved)) return [];
  const picks: { key: string; type: ConnectionType }[] = [];
  for (const key of saved) {
    const model = models.find((item) => item.key === key);
    if (model && !picks.includes(model) && pickBlock(picks.map((pick) => pick.type), model.type) === '') picks.push(model);
  }
  return picks.map((model) => model.key);
}
