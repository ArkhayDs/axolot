<script lang="ts">
  import { onMount, tick } from 'svelte';

  import Icon from '../components/Icon.svelte';
  import ModelName from '../components/ModelName.svelte';
  import Notice from '../components/Notice.svelte';
  import ChatOptions from '../components/ChatOptions.svelte';
  import ChatSources from '../components/ChatSources.svelte';
  import ChatModels from '../components/ChatModels.svelte';
  import { api, errorMessage, streamChat } from '../lib/api';
  import type { ChatDocument, ChatImage, ChatMetrics, ChatPassage, Models } from '../lib/api';
  import { askAll, restorePicks } from '../lib/chat';
  import { duration, number, seriesColor } from '../lib/format';
  import { live } from '../lib/live.svelte';

  interface Answer {
    key: string;
    model: string;
    providerId: number;
    local: boolean;
    vision: boolean | undefined;
    text: string;
    state: 'waiting' | 'streaming' | 'done' | 'failed' | 'stopped' | 'no-image';
    error: string;
    metrics: ChatMetrics | null;
  }

  interface Turn {
    question: string;
    answers: Answer[];
    searching: boolean;
    searchError: string;
    searchMs: number | null;
    passages: ChatPassage[];
    document: ChatDocument | null;
    image: ChatImage | null;
  }

  const PICKS_KEY = 'axolot.chat.models';

  let models = $state<Models | null>(null);
  let chosen = $state<string[]>([]);
  let savedPicks: unknown = null;
  try {
    savedPicks = JSON.parse(localStorage.getItem(PICKS_KEY) ?? 'null');
  } catch {
    // Storage blocked or value corrupt: start with nothing picked.
  }
  let question = $state('');
  let useDocuments = $state(false);
  let attachedDocument = $state<ChatDocument | null>(null);
  let attachedImage = $state<ChatImage | null>(null);
  let attaching = $state(false);
  let composerHeight = $state(0);
  let turns = $state<Turn[]>([]);
  let sending = $state(false);
  let error = $state('');
  let controller: AbortController | null = null;
  let thread: HTMLElement | undefined = $state();

  const embedding = $derived(new Set((models?.library ?? []).filter((entry) => entry.role === 'embedding').map((entry) => entry.name)));
  const available = $derived(
    (models?.connections ?? [])
      .filter((connection) => connection.online)
      .flatMap((connection) =>
        connection.models
          .filter((model) => !embedding.has(model.name) && !embedding.has(model.name.split(':')[0]))
          .map((model) => ({
            key: `${connection.provider.id}|${model.name}`,
            providerId: connection.provider.id,
            type: connection.connection_type,
            name: model.name,
            label: model.label,
            vision: model.vision,
          })),
      ),
  );
  // Colors go to answered then picked models only, so 4 picks never run out of the 8 colors; first seen keeps its color.
  const colorOrder = $derived([
    ...new Set([...turns.flatMap((turn) => turn.answers.map((answer) => answer.model)), ...chosen.flatMap((key) => available.find((model) => model.key === key)?.name ?? [])]),
  ]);
  const pickable = $derived(available.map((model) => ({ key: model.key, label: model.label, type: model.type, color: seriesColor(model.name, colorOrder) })));
  const blocked = $derived(live.campaign !== '');
  const imagesReadable = $derived(available.some((model) => model.vision !== false));

  /**
   * Rebuilds what one model has already said, each model keeping its own conversation
   * @param {string} key - model key
   * @returns {{ role: string; content: string }[]} - previous turns of that model
   */
  function history(key: string): { role: string; content: string }[] {
    return turns.slice(0, -1).flatMap((turn) => {
      const answer = turn.answers.find((item) => item.key === key);
      return answer && answer.state === 'done'
        ? [
            { role: 'user', content: turn.question },
            { role: 'assistant', content: answer.text },
          ]
        : [];
    });
  }

  /**
   * Asks each chosen model, local ones in turn
   * @param {SubmitEvent} event - form submit
   */
  async function send(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const text = question.trim();
    const picks = chosen.map((key) => available.find((model) => model.key === key)).filter((model) => model !== undefined);
    if (sending || !text || picks.length === 0) return;
    turns.push({
      question: text,
      answers: picks.map((model) => ({ key: model.key, model: model.name, providerId: model.providerId, local: model.type === 'local', vision: model.vision, text: '', state: 'waiting', error: '', metrics: null })),
      searching: useDocuments,
      searchError: '',
      searchMs: null,
      passages: [],
      document: attachedDocument,
      image: attachedImage,
    });
    const sentDocument = attachedDocument ? { name: attachedDocument.name, text: attachedDocument.text } : undefined;
    const turn = turns[turns.length - 1];
    const answers = turn.answers;
    question = '';
    attachedDocument = null;
    attachedImage = null;
    sending = true;
    error = '';
    controller = new AbortController();
    await tick();
    thread?.lastElementChild?.scrollIntoView({ block: 'start', behavior: 'smooth' });
    if (turn.searching) {
      try {
        const found = await api.chatSearch(text, controller.signal);
        turn.passages = found.passages;
        turn.searchMs = found.search_ms;
      } catch (searchError) {
        if (!controller.signal.aborted) {
          turn.searchError = errorMessage(searchError);
          turn.answers = [];
          turn.searching = false;
          question = text;
          attachedDocument = turn.document;
          attachedImage = turn.image;
          sending = false;
          return;
        }
      }
      turn.searching = false;
    }
    const signal = controller.signal;
    await askAll(
      answers,
      (answer) => answer.local,
      async (answer) => {
        if (signal.aborted) {
          answer.state = 'stopped';
          return;
        }
        if (turn.image && answer.vision === false) {
          answer.state = 'no-image';
          return;
        }
        answer.state = 'streaming';
        try {
          answer.metrics = await streamChat(
            { provider_id: answer.providerId, model: answer.model, messages: [...history(answer.key), { role: 'user', content: text }], passages: $state.snapshot(turn.passages), document: sentDocument, image: turn.image?.data },
            (delta) => {
              answer.text += delta;
            },
            signal,
          );
          answer.state = 'done';
        } catch (streamError) {
          if (signal.aborted) {
            answer.state = 'stopped';
          } else {
            answer.state = 'failed';
            answer.error = errorMessage(streamError);
          }
        }
      },
    );
    sending = false;
  }

  $effect(() => {
    if (!models) return;
    try {
      localStorage.setItem(PICKS_KEY, JSON.stringify(chosen));
    } catch {
      // Storage blocked: the picks only last for this visit.
    }
  });

  /** Lets Enter send and Shift+Enter add a line */
  function onKeydown(event: KeyboardEvent & { currentTarget: HTMLTextAreaElement }): void {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      event.currentTarget.form?.requestSubmit();
    }
  }

  onMount(async () => {
    try {
      models = await api.models();
      chosen = restorePicks(savedPicks, pickable);
    } catch (loadError) {
      error = errorMessage(loadError);
    }
  });
</script>

<div class="page chat">
  <div class="page-head">
    <div>
      <h1>Chat</h1>
      <p>Une question, chaque modèle choisi répond dans sa colonne : les modèles locaux à tour de rôle, ceux du cloud en même temps. Aucune instruction cachée, sauf la consigne documentaire si vous activez les documents.</p>
    </div>
    <fieldset class="models">
      <legend>
        Modèles comparés
        {#if chosen.length > 1}
          <button class="link" type="button" onclick={() => (chosen = [])}>Tout retirer</button>
        {/if}
      </legend>
      {#if available.length > 0}
        <ChatModels models={pickable} bind:chosen />
      {:else if models}
        <p class="muted">Aucun modèle en ligne. <a href="#/reglages">Ouvrir les réglages</a></p>
      {/if}
    </fieldset>
  </div>

  <Notice message={error} />

  <section class="thread" style:padding-bottom="calc({composerHeight}px + 6rem)" bind:this={thread} aria-label="Conversation" aria-live="polite">
    {#each turns as turn, index (index)}
      <article class="turn">
        <div class="question">
          <span class="who">Vous</span>
          <p>{turn.question}</p>
        </div>
        {#if turn.searching}
          <p class="muted">Recherche dans les documents...</p>
        {/if}
        {#if turn.searchError}
          <p class="state state-critical"><Icon name="alert" size={16} /> <span>{turn.searchError}. <a href="#/reglages">Ouvrir les réglages</a></span></p>
        {/if}
        <ChatSources passages={turn.passages} document={turn.document} image={turn.image} />
        <div class="answers">
          {#each turn.answers as answer (answer.key)}
            <div class="answer">
              <ModelName name={answer.model} color={seriesColor(answer.model, colorOrder)} />
              {#if turn.image && answer.vision === undefined}
                <p class="muted">Ce serveur n'indique pas si le modèle lit les images : réponse à vérifier.</p>
              {/if}
              {#if answer.state === 'no-image'}
                <p class="state state-warning"><Icon name="alert" size={16} /> Ce modèle ne lit pas les images</p>
              {:else if answer.state === 'failed'}
                <p class="state state-critical"><Icon name="cross" size={16} /> {answer.error}</p>
              {:else if answer.state === 'waiting'}
                <p class="muted">En attente de l'autre modèle...</p>
              {:else if answer.state === 'streaming' && !answer.text}
                <p class="muted">Chargement du modèle...</p>
              {:else}
                <div class="text">{answer.text}</div>
              {/if}
              {#if answer.state === 'stopped'}
                <p class="muted">Arrêté.</p>
              {/if}
              {#if answer.metrics}
                <dl class="metrics">
                  {#if turn.searchMs !== null}
                    <div><dt>Recherche</dt><dd>{duration(turn.searchMs)}</dd></div>
                  {/if}
                  <div><dt>1er token</dt><dd>{duration(answer.metrics.ttft_ms)}</dd></div>
                  <div><dt>Total</dt><dd>{duration(answer.metrics.total_ms)}</dd></div>
                  <div><dt>t/s</dt><dd>{number(answer.metrics.tok_per_sec)}</dd></div>
                  {#if answer.metrics.load_ms && answer.metrics.load_ms >= 500}
                    <div><dt>Chargement</dt><dd>{duration(answer.metrics.load_ms)}</dd></div>
                  {/if}
                </dl>
              {/if}
            </div>
          {/each}
        </div>
      </article>
    {:else}
      <p class="empty">Posez une question pour comparer le délai, le débit et la pertinence des réponses.</p>
    {/each}
  </section>

  <form class="composer glass" bind:clientHeight={composerHeight} onsubmit={send}>
    <div class="compose-row">
      <label class="field">
        <span class="visually-hidden">Votre question</span>
        <textarea class="input" rows="2" bind:value={question} onkeydown={onKeydown} placeholder={blocked ? 'Un Benchmark est en cours : le chat fausserait ses temps.' : 'Votre question'} disabled={blocked}></textarea>
      </label>
      {#if sending}
        <button class="btn" type="button" onclick={() => controller?.abort()}><Icon name="stop" /> Arrêter</button>
      {:else}
        <button class="btn btn-primary" type="submit" disabled={blocked || attaching || !question.trim() || chosen.length === 0}><Icon name="play" /> Envoyer</button>
      {/if}
    </div>
    <ChatOptions bind:useDocuments bind:document={attachedDocument} bind:image={attachedImage} bind:attaching {blocked} {imagesReadable} />
  </form>
</div>

<style>
  .models {
    flex-basis: 100%;
    min-width: 0;
    margin: 0;
    padding: 0;
    border: 0;
  }

  legend {
    margin-bottom: var(--s1);
    padding: 0;
    color: var(--ink-2);
    font-size: var(--text-sm);
  }

  .link {
    margin-left: var(--s2);
    padding: 0;
    border: 0;
    background: none;
    color: var(--ink);
    font-size: var(--text-sm);
    text-decoration: underline;
    text-decoration-color: var(--axis);
    text-underline-offset: 3px;
    cursor: pointer;
  }

  .link:hover {
    text-decoration-color: var(--accent);
  }

  .thread {
    display: grid;
    gap: var(--s6);
  }

  .turn {
    display: grid;
    gap: var(--s3);
    scroll-margin-top: 90px;
  }

  .question {
    justify-self: end;
    display: grid;
    gap: var(--s1);
    max-width: min(75ch, 85%);
    padding: var(--s3) var(--s4);
    border-radius: var(--radius-lg) var(--radius-lg) var(--radius-sm) var(--radius-lg);
    background: color-mix(in srgb, var(--accent) 14%, var(--panel));
  }

  .question p {
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }

  .who {
    color: var(--ink-3);
    font-size: var(--text-sm);
  }

  .answers {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 340px), 1fr));
    gap: var(--s3);
  }

  .answer {
    display: grid;
    align-content: start;
    gap: var(--s3);
    min-width: 0;
    padding: var(--s4);
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--panel);
  }

  .text {
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }

  .metrics {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s2) var(--s5);
    margin: 0;
    padding-top: var(--s3);
    border-top: 1px solid var(--hairline);
  }

  dt {
    color: var(--ink-3);
    font-size: var(--text-sm);
  }

  dd {
    margin: 0;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .composer {
    position: fixed;
    bottom: var(--s4);
    left: 50%;
    z-index: 5;
    display: grid;
    gap: var(--s2);
    width: min(calc(100% - 2 * var(--s5)), calc(var(--page-width) - 2 * var(--s5)));
    padding: var(--s3);
    border-radius: var(--radius-lg);
    transform: translateX(-50%);
  }

  .compose-row {
    display: flex;
    align-items: center;
    gap: var(--s3);
  }

  .compose-row .field {
    flex: 1;
  }

  textarea {
    display: block;
    resize: vertical;
  }

  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  /* The bottom navigation takes the composer's place on a phone. */
  @media (max-width: 760px) {
    .composer {
      bottom: 76px;
    }
  }
</style>
