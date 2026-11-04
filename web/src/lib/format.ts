// French display helpers for figures, statuses and scoring labels.

import type { Campaign, Check, Fit, Item } from './api';

const CATEGORIES: Record<string, string> = {
  '': 'Toutes les tâches',
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
  done: 'Terminée',
  cancelled: 'Annulée',
  interrupted: 'Interrompue',
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
  regex: 'Réponse attendue',
  max_words: 'Longueur respectée',
  retrieval: 'Bon passage retrouvé',
  citation: 'Source citée',
};

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
 * @param {Campaign['status']} status - campaign status
 * @returns {string} - display label
 */
export function statusLabel(status: Campaign['status']): string {
  return STATUSES[status];
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
  if (check.name.startsWith('json_equals:')) return `Valeur de ${check.name.slice('json_equals:'.length)}`;
  return CHECKS[check.name] ?? check.name;
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
