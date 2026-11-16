// JSON client and wire types of the Axolot API.

export interface Hardware {
  label: string;
  os: string;
  arch: string;
  chip: string;
  cores: number;
  ram_bytes: number;
  gpu: string;
  vram_bytes: number;
  detected: boolean;
}

export interface Status {
  hardware: Hardware;
  active_campaign: string;
}

export interface Settings {
  corpus_dir: string;
  chunk_size: number;
  chunk_overlap: number;
  top_k: number;
  embed_model: string;
  embed_provider_id: number;
  reserved_gb: number;
}

export interface Provider {
  id: number;
  name: string;
  kind: 'ollama' | 'openai' | 'bridge';
  base_url: string;
  has_api_key: boolean;
}

export type Fit = 'ok' | 'tight' | 'too_big' | 'unknown';

export interface Connection {
  provider: Provider;
  online: boolean;
  bridge: boolean;
  error?: string;
  // vision is absent when the server cannot tell whether the model reads images.
  models: { name: string; size_bytes: number; family?: string; quantization?: string; vision?: boolean; fit: Fit }[];
}

export interface LibraryEntry {
  name: string;
  size_bytes: number;
  role: 'chat' | 'embedding';
  fit: Fit;
}

export interface Pull {
  type: 'pull';
  provider_id: number;
  name: string;
  status: string;
  completed: number;
  total: number;
  done: boolean;
  error?: string;
}

export interface Models {
  connections: Connection[];
  library: LibraryEntry[];
  pulls: Pull[];
}

export interface Source {
  file: string;
  quote: string;
}

export interface Item {
  id: string;
  prompt: string;
  rag?: { trap: 'none' | 'absent' | 'outdated' | 'other_entity'; sources: Source[] };
}

export interface Case {
  id: string;
  title: string;
  kind: 'text' | 'decision' | 'rag';
  category: string;
  language: string;
  system?: string;
  items: Item[];
}

export interface Selection {
  case_id: string;
  provider_id: number;
  model: string;
}

export interface CasesResponse {
  cases: Case[];
  selections: Selection[];
}

export interface Campaign {
  id: string;
  name: string;
  status: 'running' | 'done' | 'cancelled' | 'interrupted';
  hardware: Hardware;
  rag_config: Settings;
  total_items: number;
  done_items: number;
  created_at: string;
  finished_at: string | null;
}

export interface Check {
  name: string;
  passed: boolean;
  detail?: string;
}

export interface Hit {
  file: string;
  position: number;
  start_offset: number;
  content: string;
  similarity: number;
}

export interface Run {
  id: string;
  campaign_id: string;
  case_id: string;
  item_id: string;
  category: string;
  provider_name: string;
  model: string;
  output: string;
  error: string;
  ttft_ms: number | null;
  total_ms: number;
  load_ms: number | null;
  cli_startup_ms: number | null;
  cli_exit_ms: number | null;
  prompt_tokens: number | null;
  output_tokens: number | null;
  tok_per_sec: number | null;
  score: number;
  checks: Check[] | null;
  retrieved: Hit[] | null;
  retrieval_hit: boolean | null;
  default_temperature: boolean;
}

export interface CaseAnswer extends Run {
  answers: number;
  avg_score: number | null;
}

export interface ModelStats {
  provider: string;
  model: string;
  category: string;
  runs: number;
  // Distinct questions behind the runs, only in the whole-database view.
  questions?: number;
  errors: number;
  // Null when no answer could be measured.
  avg_score: number | null;
  p50_ttft_ms: number | null;
  p50_total_ms: number | null;
  // Without the Claude CLI's own time, null when no answer went through the bridge.
  p50_net_ttft_ms: number | null;
  p50_net_total_ms: number | null;
  p50_tok_per_sec: number | null;
  max_load_ms: number | null;
  retrieval_hits: number;
  retrieval_total: number;
  // Set as soon as one answer ran at the model's own default temperature.
  default_temperature: boolean;
}

export interface Pick {
  model: string;
  provider: string;
  score: number;
}

export interface Verdict {
  category: string;
  quality: Pick | null;
  speed: Pick | null;
  compromise: Pick | null;
}

export interface CampaignDetail {
  campaign: Campaign;
  runs: Run[];
  stats: ModelStats[];
  verdicts: Verdict[];
}

export interface MapCell extends ModelStats {
  machine: string;
  questions: number;
  p95_total_ms: number | null;
}

export interface MapItem {
  machine: string;
  model: string;
  case_id: string;
  item_id: string;
  runs: number;
  avg_score: number | null;
  p50_total_ms: number | null;
  errors: number;
}

export interface MapBenchmark {
  id: string;
  name: string;
  status: Campaign['status'];
  created_at: string;
  finished_at: string | null;
  done_items: number;
  machine: string;
  models: string[];
  categories: string[];
  cells: { model: string; category: string; measured: number; avg_score: number | null }[];
}

export interface MapData {
  cells: MapCell[];
  items: MapItem[];
  benchmarks: MapBenchmark[];
  current_machine: string;
}

export interface Answer {
  id: string;
  item_id: string;
  model: string;
  category: string;
  total_ms: number;
  cli_startup_ms: number | null;
  cli_exit_ms: number | null;
  error: string;
}

// Empty for every type, answers of unknown type included.
export type ConnectionType = '' | 'local' | 'api' | 'subscription';

export interface BaseDetail {
  stats: MapCell[];
  verdicts: Verdict[];
  answers: Answer[];
  // Every model of the connection type, picked or not.
  models: string[];
}

export interface Estimate {
  answers: number;
  low_ms: number;
  high_ms: number;
  partial: boolean;
  known: boolean;
}

export interface CorpusResponse {
  folders: { name: string; files: string[] }[];
  indexes: { corpus: string; embed_model: string; chunks: number; files: number }[];
  ingesting: boolean;
}

/**
 * @param {unknown} error - caught value
 * @returns {string} - message safe to display
 */
export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Erreur inconnue';
}

/**
 * Calls the API and unwraps its French error message
 * @param {string} path - path below /api
 * @param {RequestInit} init - fetch options
 * @returns {Promise<Response>} - response with a 2xx status
 */
async function send(path: string, init?: RequestInit): Promise<Response> {
  let response: Response;
  try {
    response = await fetch(`/api${path}`, init);
  } catch {
    throw new Error('Serveur injoignable');
  }
  if (response.ok) return response;
  const body: { error?: string } = await response.json().catch(() => ({}));
  throw new Error(body.error ?? `Erreur ${response.status}`);
}

/**
 * Fetches and decodes a JSON resource
 * @param {string} path - path below /api
 * @returns {Promise<T>} - decoded body
 */
async function get<T>(path: string): Promise<T> {
  const response = await send(path);
  return response.json() as Promise<T>;
}

/**
 * Sends a JSON body
 * @param {string} method - HTTP verb
 * @param {string} path - path below /api
 * @param {unknown} body - value to encode
 * @returns {Promise<T>} - decoded body, undefined when the API answers empty
 */
async function write<T>(method: string, path: string, body?: unknown): Promise<T> {
  const response = await send(path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await response.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

export interface ChatMetrics {
  ttft_ms: number | null;
  total_ms: number;
  load_ms: number | null;
  output_tokens: number | null;
  tok_per_sec: number | null;
}

export interface ChatPassage {
  file: string;
  position: number;
  start_offset: number;
  content: string;
  similarity: number;
}

export interface ChatSearch {
  passages: ChatPassage[];
  search_ms: number;
}

export interface ChatDocument {
  name: string;
  text: string;
  chars: number;
  total_chars: number;
  truncated: boolean;
}

export interface ChatImage {
  name: string;
  media_type: string;
  size: number;
  data: string;
}

export interface ChatBody {
  provider_id: number;
  model: string;
  messages: { role: string; content: string }[];
  passages?: ChatPassage[];
  document?: { name: string; text: string };
  image?: string;
}

/**
 * Streams one chat answer
 * @param {{ provider_id: number; model: string; messages: { role: string; content: string }[] }} body - question and history
 * @param {(text: string) => void} onDelta - called on each fragment
 * @param {AbortSignal} signal - cancels the generation on the server too
 * @returns {Promise<ChatMetrics>} - timings of the finished answer
 */
export async function streamChat(
  body: ChatBody,
  onDelta: (text: string) => void,
  signal: AbortSignal,
): Promise<ChatMetrics> {
  const response = await send('/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  });
  if (!response.body) throw new Error('Flux non supporté');
  const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
  let buffer = '';
  for (;;) {
    const { done, value } = await reader.read();
    if (done) throw new Error('Réponse interrompue');
    buffer += value;
    const lines = buffer.split('\n');
    buffer = lines.pop() ?? '';
    for (const line of lines) {
      if (!line) continue;
      const event = JSON.parse(line) as { type: string; text?: string; error?: string } & ChatMetrics;
      if (event.type === 'delta') onDelta(event.text ?? '');
      if (event.type === 'error') throw new Error(event.error ?? 'Erreur du modèle');
      if (event.type === 'done') return event;
    }
  }
}

export const api = {
  status: (): Promise<Status> => get('/status'),
  settings: (): Promise<Settings> => get('/settings'),
  saveSettings: (settings: Settings): Promise<Settings> => write('PUT', '/settings', settings),
  providers: (): Promise<Provider[]> => get('/providers'),
  createProvider: (body: { name: string; kind: string; base_url: string; api_key: string }): Promise<{ id: number }> =>
    write('POST', '/providers', body),
  createBridge: (): Promise<{ id: number }> => write('POST', '/providers/bridge'),
  deleteProvider: (id: number): Promise<void> => write('DELETE', `/providers/${id}`),
  models: (): Promise<Models> => get('/models'),
  pullModel: (provider_id: number, name: string): Promise<void> => write('POST', '/models/pull', { provider_id, name }),
  cases: (): Promise<CasesResponse> => get('/cases'),
  caseAnswers: (id: string): Promise<CaseAnswer[]> => get(`/cases/${encodeURIComponent(id)}/answers`),
  setCaseModels: (id: string, selections: Selection[]): Promise<Selection[]> =>
    write('PUT', `/cases/${encodeURIComponent(id)}/models`, selections),
  corpus: (): Promise<CorpusResponse> => get('/corpus'),
  createCorpus: (name: string): Promise<void> => write('POST', '/corpus', { name }),
  ingestCorpus: (): Promise<void> => write('POST', '/corpus/ingest'),
  corpusFile: async (name: string, file: string): Promise<string> =>
    (await send(`/corpus/${encodeURIComponent(name)}/files/${encodeURIComponent(file)}`)).text(),
  uploadCorpusFile: async (name: string, file: File): Promise<void> => {
    const form = new FormData();
    form.append('file', file);
    await send(`/corpus/${encodeURIComponent(name)}/files`, { method: 'POST', body: form });
  },
  campaigns: (): Promise<Campaign[]> => get('/campaigns'),
  campaign: (id: string): Promise<CampaignDetail> => get(`/campaigns/${id}`),
  startCampaign: (name: string, cases: string[]): Promise<{ id: string }> => write('POST', '/campaigns', { name, cases }),
  cancelCampaign: (id: string): Promise<void> => write('POST', `/campaigns/${id}/cancel`),
  map: (): Promise<MapData> => get('/map'),
  base: (connection: ConnectionType, models: string[]): Promise<BaseDetail> => {
    const query = new URLSearchParams({ connection });
    for (const model of models) query.append('model', model);
    return get(`/base?${query}`);
  },
  estimate: (models: string[], cases: string[]): Promise<Estimate> => write('POST', '/estimate', { models, cases }),
  remaining: (): Promise<Estimate> => get('/estimate/remaining'),
  chatSearch: async (question: string, signal: AbortSignal): Promise<ChatSearch> =>
    (
      await send('/chat/search', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ question }),
        signal,
      })
    ).json() as Promise<ChatSearch>,
  chatDocument: async (file: File): Promise<ChatDocument> => {
    const form = new FormData();
    form.append('file', file);
    return (await send('/chat/document', { method: 'POST', body: form })).json() as Promise<ChatDocument>;
  },
  chatImage: async (file: File): Promise<ChatImage> => {
    const form = new FormData();
    form.append('file', file);
    return (await send('/chat/image', { method: 'POST', body: form })).json() as Promise<ChatImage>;
  },
};
