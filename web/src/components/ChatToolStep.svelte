<script lang="ts">
  import Icon from './Icon.svelte';
  import { api, errorMessage } from '../lib/api';
  import { prettyJson } from '../lib/chat';
  import type { AnswerPart } from '../lib/chat';
  import { duration } from '../lib/format';

  let { step, live }: { step: Extract<AnswerPart, { kind: 'tool' }>; live: boolean } = $props();

  let allowed = $state<boolean | null>(null);
  let sending = $state(false);
  let error = $state('');

  // A stopped answer no longer waits: its buttons would answer nothing.
  let waiting = $derived(live && step.approval !== null && step.result === null && allowed === null);

  /**
   * Sends the user's click to the call waiting for it
   * @param {boolean} allow - true for "Autoriser"
   * @param {boolean} always - "Toujours autoriser": the tool also switches to Autorisé in Permissions
   */
  async function decide(allow: boolean, always = false): Promise<void> {
    if (step.approval === null) return;
    sending = true;
    try {
      await api.answerApproval(step.approval, allow, always);
      allowed = allow;
      error = '';
    } catch (sendError) {
      error = errorMessage(sendError);
    } finally {
      sending = false;
    }
  }
</script>

<details class="step" open={waiting}>
  <summary>
    <Icon name="plug" size={16} />
    <code>{step.name}</code>
    {#if waiting}
      <span class="state-warning">en attente de validation</span>
    {:else if step.result === null}
      <span class="muted">en cours...</span>
    {:else}
      <span class="muted">{duration(step.ms)}</span>
      {#if allowed === false}
        <span class="muted">refusé</span>
      {:else if step.error}
        <span class="state-critical">échec</span>
      {/if}
    {/if}
  </summary>
  <p class="muted">Arguments</p>
  <pre>{prettyJson(step.arguments)}</pre>
  {#if step.result !== null}
    <p class="muted">Résultat</p>
    <pre>{step.result}</pre>
  {/if}
</details>
{#if waiting}
  <div class="approval">
    <button class="btn btn-primary" type="button" disabled={sending} onclick={() => decide(true)}>Autoriser</button>
    <button class="btn" type="button" disabled={sending} onclick={() => decide(true, true)}>Toujours autoriser</button>
    <button class="btn" type="button" disabled={sending} onclick={() => decide(false)}>Refuser</button>
    {#if error}<span class="state-critical">{error}</span>{/if}
  </div>
{/if}

<style>
  .step {
    display: grid;
    gap: var(--s1);
    font-size: var(--text-sm);
  }

  summary {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2);
    color: var(--ink-2);
    cursor: pointer;
  }

  code {
    font-family: var(--mono);
    overflow-wrap: anywhere;
  }

  p {
    margin: var(--s2) 0 0;
  }

  pre {
    max-height: 14rem;
    margin: var(--s1) 0 0;
    padding: var(--s3);
    overflow: auto;
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--ground);
    font-family: var(--mono);
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }

  .approval {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2);
    font-size: var(--text-sm);
  }
</style>
