// What the map holds beyond the cells: scenarios, questions and source documents from the catalog, and rankings over them.

import type { Case, Item, MapBenchmark } from '../api';
import { rankRows } from './ranking';
import type { MapFilter, MapLists, RankSection } from './ranking';
import { itemOf } from './results';
import type { ItemIndex } from './results';

export interface Question {
  key: string;
  caseId: string;
  category: string;
  item: Item;
}

export interface MapContent extends MapLists {
  scenarios: Case[];
  questions: Question[];
  // Questions citing each source file, by file name.
  documents: Map<string, Question[]>;
  benchmarks: MapBenchmark[];
}

/**
 * @param {MapLists} lists - axes the figures know
 * @param {Case[]} scenarios - catalog
 * @param {MapBenchmark[]} benchmarks - past benchmarks
 * @returns {MapContent} - everything the map draws
 */
export function contentOf(lists: MapLists, scenarios: Case[], benchmarks: MapBenchmark[]): MapContent {
  const questions = scenarios.flatMap((scenario) =>
    scenario.items.map((item): Question => ({ key: `${scenario.id}/${item.id}`, caseId: scenario.id, category: scenario.category, item })),
  );
  const documents = new Map<string, Question[]>();
  for (const question of questions) {
    for (const file of new Set((question.item.rag?.sources ?? []).map((source) => source.file))) {
      documents.set(file, [...(documents.get(file) ?? []), question]);
    }
  }
  return {
    machines: lists.machines,
    models: lists.models,
    themes: [...new Set([...lists.themes, ...scenarios.map((scenario) => scenario.category)])].sort(),
    scenarios,
    questions,
    documents,
    benchmarks,
  };
}

/**
 * Ranks models on a set of questions: score per question averaged, as everywhere; a median of medians would be an invented figure, so times stay out
 * @param {ItemIndex} items - cumulated figures
 * @param {string[]} models - models to rank
 * @param {string} machine - machine label, empty for all
 * @param {Question[]} questions - questions of a scenario or document
 * @returns {RankSection[]} - one ranking of models
 */
export function rankOnQuestions(items: ItemIndex, models: string[], machine: string, questions: Question[]): RankSection[] {
  const entries = models.map((model) => {
    const played = questions.flatMap((question) => itemOf(items, machine, model, question.caseId, question.item.id) ?? []);
    const scores = played.flatMap((item) => item.avg_score ?? []);
    const figures =
      played.length === 0
        ? undefined
        : {
            avg_score: scores.length === 0 ? null : scores.reduce((total, value) => total + value, 0) / scores.length,
            runs: played.reduce((total, item) => total + item.runs, 0),
            questions: played.length,
            errors: played.reduce((total, item) => total + item.errors, 0),
            p50_total_ms: null,
            p95_total_ms: null,
            p50_tok_per_sec: null,
          };
    return { key: model, label: model, cell: figures };
  });
  return [{ kind: 'model', title: 'Modèles', rows: rankRows('model', entries, '') }];
}

/**
 * @param {Question} question - question
 * @param {MapFilter} filter - current selection
 * @returns {boolean} - whether the theme filter keeps it
 */
export function questionKept(question: Question, filter: MapFilter): boolean {
  return !filter.category || question.category === filter.category;
}
