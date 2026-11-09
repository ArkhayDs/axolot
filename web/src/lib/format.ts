// French display helpers for figures, statuses and scoring labels.

import type { Campaign, Check, Estimate, Fit, Item } from './api';

const CATEGORIES: Record<string, string> = {
  '': 'Tous les thèmes',
  ambiguite: 'Questions ambiguës',
  calcul: 'Calcul',
  lecture: 'Lecture de documents',
  code: 'Code',
  rag: 'Documents internes',
  extraction: 'Extraction',
  resume: 'Résumé',
  routage: 'Routage',
  tri: 'Tri',
};

const STATUSES: Record<Campaign['status'], string> = {
  running: 'En cours',
  done: 'Terminé',
  cancelled: 'Annulé',
  interrupted: 'Interrompu',
};

const FITS: Record<Fit, string> = { ok: 'OK', tight: 'Juste', too_big: 'Trop gros', unknown: 'Inconnu' };

const TRAPS: Record<NonNullable<Item['rag']>['trap'], string> = {
  none: 'Sans piège',
  absent: 'Réponse absente',
  outdated: 'Version périmée',
  other_entity: 'Autre entité',
};

const CHECKS: Record<string, string> = {
  json_valid: 'JSON valide',
  json_required: 'Champs requis',
  contains_all: 'Contient l’attendu',
  contains_any: 'Contient l’attendu',
  not_contains: 'Évite l’interdit',
  regex: 'Contenu attendu',
  max_words: 'Longueur respectée',
  retrieval: 'Bon passage retrouvé',
  citation: 'Source citée',
};

const FIELDS: Record<string, string> = {
  decision: 'la décision',
  cible: 'la cible',
  reponse: 'la réponse',
  categorie: 'la catégorie',
  urgence: "l'urgence",
};

const VALUES: Record<string, string> = { executer: 'exécuter', demander_precision: 'demander une précision' };

export const UNMEASURED_HINT = "La question n'a pas pu être mesurée (connexion, quota, délai) : elle ne compte pas dans la moyenne.";

const decimal = new Intl.NumberFormat('fr-FR', { maximumFractionDigits: 1 });
const dateTime = new Intl.DateTimeFormat('fr-FR', { dateStyle: 'medium', timeStyle: 'short' });

/**
 * @param {string} category - case category, empty for overall
 * @returns {string} - display label, the raw name for an unmapped category
 */
export function categoryLabel(category: string): string {
  return CATEGORIES[category] ?? category.charAt(0).toUpperCase() + category.slice(1);
}

/**
 * @param {number} errors - answers lost to an execution error
 * @returns {string} - count with its label, e.g. "2 non mesurées"
 */
export function unmeasuredLabel(errors: number): string {
  return `${errors} non mesurée${errors > 1 ? 's' : ''}`;
}

/**
 * @param {Campaign['status']} status - campaign status
 * @returns {string} - display label
 */
export function statusLabel(status: Campaign['status']): string {
  return STATUSES[status];
}

/**
 * Names saved before the rename still read "Campagne" in the database
 * @param {string} name - benchmark name
 * @returns {string} - display name
 */
export function benchmarkName(name: string): string {
  return name.replace(/^Campagne\b/, 'Benchmark');
}

/**
 * @param {Estimate} estimate - range sent by the API
 * @returns {string} - range in whole minutes; never a figure when something could not be estimated
 */
export function estimateLabel(estimate: Estimate): string {
  if (!estimate.known) return 'Durée inconnue';
  const low = Math.round(estimate.low_ms / 60_000);
  const high = Math.round(estimate.high_ms / 60_000);
  if (high < 1) return "moins d'une minute";
  return low === high || low < 1 ? `environ ${high} min` : `environ ${low} à ${high} min`;
}

/**
 * @param {Fit} fit - hardware fit verdict
 * @returns {string} - display label
 */
export function fitLabel(fit: Fit): string {
  return FITS[fit];
}

/**
 * @param {NonNullable<Item['rag']>['trap']} trap - RAG trap type
 * @returns {string} - display label
 */
export function trapLabel(trap: NonNullable<Item['rag']>['trap']): string {
  return TRAPS[trap];
}

/**
 * @param {Check} check - scoring check
 * @returns {string} - display label
 */
export function checkLabel(check: Check): string {
  if (check.name.startsWith('json_equals:')) {
    const field = check.name.slice('json_equals:'.length);
    return `Valeur de ${FIELDS[field] ?? field}`;
  }
  return CHECKS[check.name] ?? check.name;
}

/**
 * Translates the scoring engine's technical detail for display
 * @param {string} detail - detail sent by the API
 * @returns {string} - French text, unchanged when unknown
 */
export function checkDetail(detail: string): string {
  const mismatch = /^got (.*), want (.*)$/.exec(detail);
  if (mismatch) return `obtenu ${VALUES[mismatch[1]] ?? (mismatch[1] || '(vide)')}, attendu ${VALUES[mismatch[2]] ?? mismatch[2]}`;
  const length = /^(\d+) words, limit (.*)$/.exec(detail);
  if (length) return `${length[1]} mots, limite ${length[2]}`;
  return detail
    .replace(/^missing: /, 'manquant : ')
    .replace(/^none of: /, 'aucun de : ')
    .replace(/^found: /, 'trouvé : ')
    .replace('invalid json', 'JSON invalide')
    .replace('invalid schema', 'schéma invalide')
    .replace('not an object', 'pas un objet')
    .replace('path not found', 'champ absent')
    .replace('expected passage retrieved', 'passage attendu absent des extraits')
    .replace('expected source not cited', 'source attendue non citée');
}

/**
 * @param {number | null | undefined} ms - duration
 * @returns {string} - seconds under a minute, then minutes
 */
export function duration(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return '-';
  if (ms < 60_000) return `${decimal.format(ms / 1000)} s`;
  const seconds = Math.round(ms / 1000);
  return `${Math.floor(seconds / 60)} min ${String(seconds % 60).padStart(2, '0')}`;
}

/**
 * @param {number | null | undefined} value - any figure
 * @returns {string} - French number, one decimal at most
 */
export function number(value: number | null | undefined): string {
  return value === null || value === undefined ? '-' : decimal.format(value);
}

/**
 * Keeps a decimal when rounding would make two different scores look tied
 * @param {number} score - score in 0-100
 * @returns {string} - display score
 */
export function score(score: number): string {
  return decimal.format(score);
}

/**
 * @param {number} bytes - size
 * @returns {string} - decimal gigabytes, like the sizes announced by the model library
 */
export function gigabytes(bytes: number): string {
  return `${decimal.format(bytes / 1e9)} Go`;
}

/**
 * Splits a prompt into what the model is told and what it is asked: the request is the last paragraph
 * @param {string} prompt - full prompt
 * @returns {{ context: string; request: string }} - context, empty for a one-paragraph prompt
 */
export function splitPrompt(prompt: string): { context: string; request: string } {
  const cut = prompt.trimEnd().lastIndexOf('\n\n');
  if (cut < 0) return { context: '', request: prompt.trim() };
  const context = prompt.slice(0, cut).replace(/^Contexte : /, '').trim();
  return { context: context.charAt(0).toUpperCase() + context.slice(1), request: prompt.slice(cut).trim() };
}

/**
 * @param {string} iso - timestamp
 * @returns {string} - French date and time
 */
export function when(iso: string): string {
  return dateTime.format(new Date(iso));
}

/**
 * @param {string} model - model tag
 * @param {string[]} all - every model of the view, sorted, so a model keeps its color when a filter hides another
 * @returns {string} - CSS color token
 */
export function seriesColor(model: string, all: string[]): string {
  const index = all.indexOf(model);
  // shortcut: past 8 models the rest share a neutral color, fold them into "Autres" if a campaign ever gets there.
  return index >= 0 && index < 8 ? `var(--series-${index + 1})` : 'var(--series-other)';
}
