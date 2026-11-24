// Chat model picks: each model gets its own answer column.

import type { ChatEvent, ConnectionType } from './api';

export const MAX_MODELS = 4;
export const MAX_LOCAL = 2;

// A tool part's result is null while the call runs.
export type AnswerPart =
  | { kind: 'text'; text: string }
  | { kind: 'tool'; name: string; arguments: string; result: string | null; error: boolean; ms: number | null };

/**
 * Adds one streamed event to an answer, text and tool steps kept in the order the model produced them
 * @param {AnswerPart[]} parts - answer so far, changed in place
 * @param {ChatEvent} event - delta or tool step; notices are not parts
 */
export function addEvent(parts: AnswerPart[], event: ChatEvent): void {
  const last = parts.at(-1);
  if (event.type === 'delta') {
    if (last?.kind === 'text') last.text += event.text;
    else parts.push({ kind: 'text', text: event.text });
  } else if (event.type === 'tool_call') {
    parts.push({ kind: 'tool', name: event.name, arguments: event.arguments, result: null, error: false, ms: null });
  } else if (event.type === 'tool_result') {
    if (last?.kind !== 'tool' || last.result !== null) throw new Error("Résultat d'outil reçu sans son appel");
    last.result = event.result;
    last.error = event.error;
    last.ms = event.ms;
  }
}

/**
 * What the model wrote, tool steps left out, for the next turn's history
 * @param {AnswerPart[]} parts - finished answer
 * @returns {string} - text parts joined by a blank line
 */
export function answerText(parts: AnswerPart[]): string {
  return parts.flatMap((part) => (part.kind === 'text' ? [part.text] : [])).join('\n\n');
}

/**
 * Tool calls of an answer and the time spent in them
 * @param {AnswerPart[]} parts - answer so far
 * @returns {{ calls: number; ms: number }} - calls made, finished ones' total duration
 */
export function toolSummary(parts: AnswerPart[]): { calls: number; ms: number } {
  const tools = parts.filter((part) => part.kind === 'tool');
  return { calls: tools.length, ms: tools.reduce((sum, part) => sum + (part.ms ?? 0), 0) };
}

/**
 * Indents tool arguments for reading
 * @param {string} text - model's raw JSON, possibly invalid
 * @returns {string} - indented JSON, or the text as is
 */
export function prettyJson(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

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
