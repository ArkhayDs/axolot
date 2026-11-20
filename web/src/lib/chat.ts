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
 * Asks every model: local ones in turn (one GPU, sharing it would falsify their timings), the others alongside
 * @param {T[]} models - picked models, in answer order
 * @param {(model: T) => boolean} local - whether a model runs on the local GPU
 * @param {(model: T) => Promise<void>} ask - gets one answer, never rejects
 */
export async function askAll<T>(models: T[], local: (model: T) => boolean, ask: (model: T) => Promise<void>): Promise<void> {
  const chain = async (): Promise<void> => {
    for (const model of models.filter(local)) await ask(model);
  };
  await Promise.all([chain(), ...models.filter((model) => !local(model)).map(ask)]);
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

/**
 * Picks to store: current ones, then saved ones offline this visit, so a model down at load is not forgotten
 * @param {string[]} chosen - current picks
 * @param {unknown} saved - parsed stored value, untrusted
 * @param {string[]} online - keys of the models online
 * @returns {string[]} - keys to store
 */
export function picksToSave(chosen: string[], saved: unknown, online: string[]): string[] {
  if (!Array.isArray(saved)) return chosen;
  return [...chosen, ...saved.filter((key) => typeof key === 'string' && !online.includes(key) && !chosen.includes(key))];
}
