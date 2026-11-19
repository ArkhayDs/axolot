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
