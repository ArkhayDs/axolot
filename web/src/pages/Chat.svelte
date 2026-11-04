<script lang="ts">
  import { onMount, tick } from 'svelte';

  import Icon from '../components/Icon.svelte';
  import ModelName from '../components/ModelName.svelte';
  import Notice from '../components/Notice.svelte';
  import { api, errorMessage, streamChat } from '../lib/api';
  import type { ChatMetrics, Models } from '../lib/api';
  import { duration, number, seriesColor } from '../lib/format';
  import { live } from '../lib/live.svelte';

  interface Answer {
    key: string;
    model: string;
    providerId: number;
    text: string;
    state: 'waiting' | 'streaming' | 'done' | 'failed' | 'stopped';
    error: string;
    metrics: ChatMetrics | null;
  }

  interface Turn {
    question: string;
    answers: Answer[];
  }

  let models = $state<Models | null>(null);
  let chosen = $state<string[]>([]);
  let question = $state('');
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
          .map((model) => ({ key: `${connection.provider.id}|${model.name}`, providerId: connection.provider.id, name: model.name })),
      ),
  );
  const colorOrder = $derived(available.map((model) => model.name).sort());
  const blocked = $derived(live.campaign !== '');

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
   * Asks each chosen model in turn: one GPU, so answering together would falsify the timings
   * @param {SubmitEvent} event - form submit
   */
  async function send(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const text = question.trim();
    const picks = available.filter((model) => chosen.includes(model.key));
    if (!text || picks.length === 0) return;
    turns.push({
      question: text,
      answers: picks.map((model) => ({ key: model.key, model: model.name, providerId: model.providerId, text: '', state: 'waiting', error: '', metrics: null })),
    });
    const answers = turns[turns.length - 1].answers;
    question = '';
    sending = true;
    error = '';
    controller = new AbortController();
    await tick();
    thread?.lastElementChild?.scrollIntoView({ block: 'start', behavior: 'smooth' });
    for (const answer of answers) {
      if (controller.signal.aborted) {
        answer.state = 'stopped';
        continue;
      }
      answer.state = 'streaming';
      try {
        answer.metrics = await streamChat(
          { provider_id: answer.providerId, model: answer.model, messages: [...history(answer.key), { role: 'user', content: text }] },
          (delta) => {
            answer.text += delta;
          },
          controller.signal,
        );
        answer.state = 'done';
      } catch (streamError) {
        if (controller.signal.aborted) {
          answer.state = 'stopped';
        } else {
          answer.state = 'failed';
          answer.error = errorMessage(streamError);
        }
      }
    }
    sending = false;
  }

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
      chosen = available.map((model) => model.key);
    } catch (loadError) {
      error = errorMessage(loadError);
    }
  });
</script>

<div class="page chat">
  <div class="page-head">
    <div>
      <h1>Chat</h1>
      <p>Une question, chaque modèle coché répond à son tour, sans instruction cachée.</p>
    </div>
    <fieldset class="models">
      <legend>Modèles</legend>
      {#each available as model (model.key)}
        <label class="chip" class:on={chosen.includes(model.key)}>
          <input type="checkbox" bind:group={chosen} value={model.key} />
          <ModelName name={model.name} color={seriesColor(model.name, colorOrder)} />
        </label>
      {:else}
        {#if models}<p class="muted">Aucun modèle en ligne. <a href="#/reglages">Ouvrir les réglages</a></p>{/if}
      {/each}
    </fieldset>
  </div>

  <Notice message={error} />

  <section class="thread" bind:this={thread} aria-label="Conversation" aria-live="polite">
    {#each turns as turn, index (index)}
      <article class="turn">
        <p class="question">{turn.question}</p>
        <div class="answers">
          {#each turn.answers as answer (answer.key)}
            <div class="answer">
              <ModelName name={answer.model} color={seriesColor(answer.model, colorOrder)} />
              {#if answer.state === 'failed'}
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
                  <div><dt>1er jeton</dt><dd>{duration(answer.metrics.ttft_ms)}</dd></div>
                  <div><dt>Total</dt><dd>{duration(answer.metrics.total_ms)}</dd></div>
                  <div><dt>Jetons/s</dt><dd>{number(answer.metrics.tok_per_sec)}</dd></div>
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

  <form class="composer glass" onsubmit={send}>
    <label class="field">
      <span class="visually-hidden">Votre question</span>
      <textarea class="input" rows="2" bind:value={question} onkeydown={onKeydown} placeholder={blocked ? 'Une campagne est en cours : le chat fausserait ses temps.' : 'Votre question'} disabled={blocked}></textarea>
    </label>
    {#if sending}
      <button class="btn" type="button" onclick={() => controller?.abort()}><Icon name="stop" /> Arrêter</button>
    {:else}
      <button class="btn btn-primary" type="submit" disabled={blocked || !question.trim() || chosen.length === 0}><Icon name="play" /> Envoyer</button>
    {/if}
  </form>
</div>

<style>
  .models {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2);
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

  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--s2);
    min-height: 40px;
    padding: var(--s1) var(--s3);
    border: 1px solid var(--axis);
    border-radius: var(--radius);
    cursor: pointer;
  }

  .chip.on {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 14%, transparent);
  }

  .thread {
    display: grid;
    gap: var(--s6);
    /* The composer floats over the page bottom: the last answer must clear it. */
    padding-bottom: 7rem;
  }

  .turn {
    display: grid;
    gap: var(--s3);
    scroll-margin-top: 90px;
  }

  .question {
    max-width: 75ch;
    font-size: var(--text-lg);
    font-weight: 550;
    white-space: pre-wrap;
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
    display: flex;
    align-items: center;
    gap: var(--s3);
    width: min(calc(100% - 2 * var(--s5)), calc(var(--page-width) - 2 * var(--s5)));
    padding: var(--s3);
    border-radius: var(--radius-lg);
    transform: translateX(-50%);
  }

  .composer .field {
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

    .thread {
      padding-bottom: 9rem;
    }
  }
</style>
