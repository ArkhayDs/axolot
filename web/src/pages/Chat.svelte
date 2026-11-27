<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';

  import ChatApproval from '../components/ChatApproval.svelte';
  import Icon from '../components/Icon.svelte';
  import ModelName from '../components/ModelName.svelte';
  import Notice from '../components/Notice.svelte';
  import ChatOptions from '../components/ChatOptions.svelte';
  import ChatSources from '../components/ChatSources.svelte';
  import ChatModels from '../components/ChatModels.svelte';
  import ChatToolRun from '../components/ChatToolRun.svelte';
  import { api, errorMessage, streamChat } from '../lib/api';
  import type { ChatDocument, ChatImage, ChatMetrics, ChatPassage, Models } from '../lib/api';
  import { addEvent, answerText, askAll, awaitsUser, picksToSave, restorePicks, toolRuns, toolSummary } from '../lib/chat';
  import type { AnswerPart, ToolPart } from '../lib/chat';
  import { BUFFERED_HINT, STARTUP_HINT, duration, number, seriesColor } from '../lib/format';
  import { live } from '../lib/live.svelte';

  interface Answer {
    key: string;
    model: string;
    providerId: number;
    local: boolean;
    vision: boolean | undefined;
    parts: AnswerPart[];
    notices: string[];
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
  // Approval ids whose buttons are off screen (none until the observer says so, or the bar flashes); 76 px is the composer's largest bottom offset (mobile).
  const approvalsOffScreen = new SvelteSet<string>();
  let hiddenApprovals = $derived(
    turns.flatMap((turn) =>
      turn.answers.flatMap((answer) =>
        answer.parts
          .filter((part): part is ToolPart => part.kind === 'tool' && awaitsUser(part, answer.state === 'streaming') && approvalsOffScreen.has(part.approval ?? ''))
          .map((step) => ({ model: answer.model, step })),
      ),
    ),
  );
  let sending = $state(false);
  let error = $state('');
  let controller: AbortController | null = null;
  let thread: HTMLElement | undefined = $state();
  // The page follows the conversation's end while the user stays there; scrolling up stops it, coming back down resumes it.
  let following = true;
  let lastScrollY = 0;

  /** Keeps following unless the user scrolled up away from the end (a smooth scroll down passes through mid-page). */
  function onScroll() {
    const atEnd = innerHeight + scrollY >= document.documentElement.scrollHeight - 80;
    following = atEnd || (following && scrollY >= lastScrollY);
    lastScrollY = scrollY;
  }

  $effect(() => {
    if (!thread) return;
    const observer = new ResizeObserver(() => {
      if (following) scrollTo({ top: document.documentElement.scrollHeight });
    });
    observer.observe(thread);
    return () => observer.disconnect();
  });

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
  const colorOrder = $derived([...new Set([...turns.flatMap((turn) => turn.answers.map((answer) => answer.key)), ...chosen])]);
  const pickable = $derived(available.map((model) => ({ key: model.key, label: model.label, type: model.type, color: seriesColor(model.key, colorOrder) })));
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
      return answer && answer.state === 'done' && answerText(answer.parts).trim()
        ? [
            { role: 'user', content: turn.question },
            { role: 'assistant', content: answerText(answer.parts) },
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
      answers: picks.map((model) => ({ key: model.key, model: model.name, providerId: model.providerId, local: model.type === 'local', vision: model.vision, parts: [], notices: [], state: 'waiting', error: '', metrics: null })),
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
    following = true;
    scrollTo({ top: document.documentElement.scrollHeight, behavior: 'smooth' });
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
    try {
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
              (event) => {
                if (event.type === 'notice') answer.notices.push(event.text);
                else addEvent(answer.parts, event);
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
    } finally {
      sending = false;
    }
  }

  $effect(() => {
    if (!models) return;
    try {
      localStorage.setItem(PICKS_KEY, JSON.stringify(picksToSave(chosen, savedPicks, available.map((model) => model.key))));
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

<svelte:window onscroll={onScroll} />

{#snippet pending(text: string, tone: string)}
  <p class={tone}>{text}<span class="dots" aria-hidden="true"><span>.</span><span>.</span><span>.</span></span></p>
{/snippet}

<div class="page chat">
  <div class="page-head">
    <div>
      <h1>Chat</h1>
      <p>Une question, chaque modèle choisi répond dans sa colonne : les modèles locaux à tour de rôle, ceux du cloud en même temps. Aucune instruction cachée, sauf la consigne documentaire si vous activez les documents et la liste des outils MCP que vous avez bloqués.</p>
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
              <ModelName name={answer.model} color={seriesColor(answer.key, colorOrder)} />
              {#if turn.image && answer.vision === undefined}
                <p class="muted">Ce serveur n'indique pas si le modèle lit les images : réponse à vérifier.</p>
              {/if}
              {#each answer.notices as notice, index (index)}
                <p class="state state-warning"><Icon name="alert" size={16} /> {notice}</p>
              {/each}
              {#if answer.state === 'no-image'}
                <p class="state state-warning"><Icon name="alert" size={16} /> Ce modèle ne lit pas les images</p>
              {:else if answer.state === 'waiting'}
                <p class="muted">En attente de l'autre modèle...</p>
              {:else if answer.state === 'streaming' && answer.parts.length === 0}
                {@render pending('Chargement du modèle', 'muted')}
              {/if}
              {#each toolRuns(answer.parts) as block, index (index)}
                {#if Array.isArray(block)}
                  <ChatToolRun
                    steps={block}
                    live={answer.state === 'streaming'}
                    bottomInset={composerHeight + 76}
                    onseen={(id, seen) => (seen ? approvalsOffScreen.delete(id) : approvalsOffScreen.add(id))}
                  />
                {:else if block.kind === 'text'}
                  <div class="text">{block.text}</div>
                {:else if block.ms !== null}
                  <p class="musing muted">{block.word} pendant {duration(block.ms)}</p>
                {:else if answer.state === 'streaming'}
                  {@render pending(block.word, 'musing live')}
                {/if}
              {/each}
              {#if answer.state === 'failed'}
                <p class="state state-critical"><Icon name="cross" size={16} /> {answer.error}</p>
              {/if}
              {#if answer.state === 'stopped'}
                <p class="muted">Arrêté.</p>
              {/if}
              {#if answer.metrics}
                {@const tools = toolSummary(answer.parts)}
                {@const startup = answer.metrics.load_ms ?? answer.metrics.cli_startup_ms}
                <dl class="metrics">
                  {#if turn.searchMs !== null}
                    <div><dt>Recherche</dt><dd>{duration(turn.searchMs)}</dd></div>
                  {/if}
                  <div><dt>1er token</dt><dd>{duration(answer.metrics.ttft_ms)}</dd></div>
                  <div><dt>Total</dt><dd>{duration(answer.metrics.total_ms)}</dd></div>
                  <div>
                    <dt>t/s</dt>
                    <dd>
                      {number(answer.metrics.tok_per_sec)}
                      {#if answer.metrics.tok_per_sec === null && answer.metrics.cli_startup_ms !== null && tools.calls > 0}
                        <span class="hint" role="img" aria-label={BUFFERED_HINT} title={BUFFERED_HINT}><Icon name="info" size={14} /></span>
                      {/if}
                    </dd>
                  </div>
                  {#if startup !== null}
                    <div><dt title={STARTUP_HINT}>Démarrage</dt><dd>{duration(startup)}</dd></div>
                  {/if}
                  {#if tools.calls > 0}
                    <div><dt>Outils</dt><dd>{tools.calls} {tools.calls > 1 ? 'appels' : 'appel'} · {duration(tools.ms)}</dd></div>
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
    {#if hiddenApprovals.length > 0}
      <ul class="approvals" aria-label="Appels en attente de votre accord">
        {#each hiddenApprovals as { model, step } (step.approval)}
          <li>
            <span><strong>{model}</strong> attend votre accord · <code>{step.name}</code></span>
            <button class="btn" type="button" onclick={() => document.getElementById(`approval-${step.approval}`)?.scrollIntoView({ block: 'center', behavior: 'smooth' })}>Voir</button>
            <ChatApproval {step} />
          </li>
        {/each}
      </ul>
    {/if}
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
    display: flex;
    flex-direction: column;
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

  .musing {
    font-size: var(--text-sm);
  }

  .live {
    color: var(--accent);
  }

  .dots span {
    animation: dot 1.2s ease-in-out infinite both;
  }

  .dots span:nth-child(2) {
    animation-delay: 0.2s;
  }

  .dots span:nth-child(3) {
    animation-delay: 0.4s;
  }

  @keyframes dot {
    0%,
    60%,
    100% {
      opacity: 0.2;
    }

    30% {
      opacity: 1;
    }
  }

  .metrics {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s2) var(--s5);
    margin: auto 0 0;
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

  .hint {
    display: inline-flex;
    vertical-align: -2px;
    color: var(--ink-3);
    cursor: help;
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

  .approvals {
    display: grid;
    gap: var(--s2);
    margin: 0;
    padding: 0 0 var(--s3);
    border-bottom: 1px solid var(--hairline);
    list-style: none;
    font-size: var(--text-sm);
  }

  .approvals li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2) var(--s3);
  }

  .approvals span {
    flex: 1 1 16rem;
  }

  .approvals code {
    font-family: var(--mono);
    overflow-wrap: anywhere;
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
