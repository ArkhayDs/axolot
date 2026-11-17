// Deterministic quality indicators computed from stored answers alone: no model, no global grade.

import type { Run } from './api';

export interface QualityRow {
  model: string;
  // Empty for the model across all themes.
  category: string;
  answers: number;
  wordsMedian: number | null;
  wordsP95: number | null;
  sentenceWords: number | null;
  diversity: number | null;
  jsonRate: number | null;
  citationRate: number | null;
  blankRate: number;
}

export const INDICATORS: { label: string; hint: string }[] = [
  { label: 'Mots, médiane et p95', hint: 'Longueur des réponses non vides, en mots.' },
  { label: 'Mots par phrase', hint: 'Moyenne sur les réponses en texte, JSON exclu.' },
  { label: 'Diversité lexicale', hint: 'Mots distincts sur mots, en moyenne par réponse en texte. Baisse quand une réponse se répète.' },
  { label: 'JSON valide', hint: 'Part des réponses qui se lisent en JSON, là où un schéma est attendu.' },
  { label: 'Source citée', hint: 'Part des réponses qui citent la source attendue, sur les questions documentaires.' },
  { label: 'Vides ou tronquées', hint: 'Part des réponses sans texte, ou arrêtées par la limite de tokens.' },
];

/**
 * @param {string} text - answer
 * @returns {string[]} - words, numbers included, lowercased
 */
export function wordsOf(text: string): string[] {
  return (text.toLowerCase().match(/[\p{L}\p{N}]+(?:['’-][\p{L}\p{N}]+)*/gu) ?? []);
}

/**
 * @param {string} text - answer
 * @returns {string[]} - sentences holding at least one word; a decimal point does not end one
 */
export function sentencesOf(text: string): string[] {
  return text.split(/[.!?…]+(?=\s|$)|\n+/u).filter((part) => wordsOf(part).length > 0);
}

/**
 * @param {number[]} values - figures
 * @param {number} ratio - percentile in 0-1
 * @returns {number | null} - interpolated percentile, like the database's percentile_cont
 */
export function percentile(values: number[], ratio: number): number | null {
  if (values.length === 0) return null;
  const sorted = [...values].sort((a, b) => a - b);
  const position = (sorted.length - 1) * ratio;
  const below = Math.floor(position);
  const above = Math.min(below + 1, sorted.length - 1);
  return sorted[below] + (sorted[above] - sorted[below]) * (position - below);
}

/**
 * @param {number[]} values - figures
 * @returns {number | null} - mean, null without data
 */
function mean(values: number[]): number | null {
  return values.length === 0 ? null : values.reduce((total, value) => total + value, 0) / values.length;
}

/**
 * @param {Run[]} runs - answers
 * @param {string} name - scoring check
 * @returns {number | null} - share passing it, null where it does not apply
 */
function checkRate(runs: Run[], name: string): number | null {
  const checks = runs.flatMap((run) => (run.checks ?? []).filter((check) => check.name === name));
  return mean(checks.map((check) => (check.passed ? 1 : 0)));
}

/**
 * @param {string} model - model tag
 * @param {string} category - theme, empty for all
 * @param {Run[]} runs - answers of this pair
 * @param {number} maxTokens - output limit the runner applies
 * @returns {QualityRow} - indicators
 */
function rowOf(model: string, category: string, runs: Run[], maxTokens: number): QualityRow {
  const written = runs.filter((run) => run.output.trim() !== '');
  const lengths = written.map((run) => wordsOf(run.output).length);
  const prose = written.filter((run) => !(run.checks ?? []).some((check) => check.name === 'json_valid'));
  const sentenceLengths = prose.flatMap((run) => sentencesOf(run.output).map((sentence) => wordsOf(sentence).length));
  const diversities = prose.map((run) => {
    const words = wordsOf(run.output);
    return words.length === 0 ? 0 : new Set(words).size / words.length;
  });
  const blank = runs.filter((run) => run.output.trim() === '' || (run.output_tokens ?? 0) >= maxTokens);
  return {
    model,
    category,
    answers: runs.length,
    wordsMedian: percentile(lengths, 0.5),
    wordsP95: percentile(lengths, 0.95),
    sentenceWords: mean(sentenceLengths),
    diversity: mean(diversities),
    jsonRate: checkRate(runs, 'json_valid'),
    citationRate: checkRate(runs, 'citation'),
    blankRate: runs.length === 0 ? 0 : blank.length / runs.length,
  };
}

/**
 * @param {Run[]} runs - answers of a benchmark
 * @param {number} maxTokens - output limit the runner applies
 * @returns {QualityRow[]} - one row per model overall, then per model and theme
 */
export function qualityRows(runs: Run[], maxTokens: number): QualityRow[] {
  const models = [...new Set(runs.map((run) => run.label))].sort();
  const themes = [...new Set(runs.map((run) => run.category))].sort();
  const overall = models.map((model) => rowOf(model, '', runs.filter((run) => run.label === model), maxTokens));
  const detailed = themes.flatMap((category) =>
    models.flatMap((model) => {
      const pair = runs.filter((run) => run.label === model && run.category === category);
      return pair.length > 0 ? [rowOf(model, category, pair, maxTokens)] : [];
    }),
  );
  return [...overall, ...detailed];
}
