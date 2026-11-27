<script lang="ts">
  import { api, errorMessage } from '../lib/api';
  import type { ToolPart } from '../lib/chat';

  let { step }: { step: ToolPart } = $props();

  let sending = $state(false);
  let error = $state('');

  /**
   * Sends the user's click to the call waiting for it; the step is marked decided for every place showing these buttons
   * @param {boolean} allow - true for "Autoriser"
   * @param {boolean} always - "Toujours autoriser": the tool also switches to Autorisé in Permissions
   */
  async function decide(allow: boolean, always = false): Promise<void> {
    if (step.approval === null) return;
    sending = true;
    try {
      await api.answerApproval(step.approval, allow, always);
      step.decided = true;
      error = '';
    } catch (sendError) {
      error = errorMessage(sendError);
    } finally {
      sending = false;
    }
  }
</script>

<div class="approval">
  <button class="btn btn-primary" type="button" disabled={sending} onclick={() => decide(true)}>Autoriser</button>
  <button class="btn" type="button" disabled={sending} onclick={() => decide(true, true)}>Toujours autoriser</button>
  <button class="btn" type="button" disabled={sending} onclick={() => decide(false)}>Refuser</button>
  {#if error}<span class="state-critical">{error}</span>{/if}
</div>

<style>
  .approval {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s2);
    font-size: var(--text-sm);
  }
</style>
