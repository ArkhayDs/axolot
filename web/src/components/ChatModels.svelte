<script lang="ts">
  import Icon from './Icon.svelte';
  import ModelName from './ModelName.svelte';
  import type { ConnectionType } from '../lib/api';
  import { MAX_LOCAL, MAX_MODELS, pickBlock } from '../lib/chat';
  import { CONNECTIONS } from '../lib/format';

  interface Pickable {
    key: string;
    label: string;
    type: ConnectionType;
    color: string;
  }

  let { models, chosen = $bindable() }: { models: Pickable[]; chosen: string[] } = $props();

  let open = $state(false);

  // A picked model gone offline drops out of the slots without losing its key.
  const picks = $derived(chosen.map((key) => models.find((model) => model.key === key)).filter((model) => model !== undefined));
  const types = $derived(picks.map((model) => model.type));
  const columns = $derived(
    CONNECTIONS.map((option) => ({ type: option.value, title: option.label, models: models.filter((model) => model.type === option.value) })).filter(
      (column) => column.models.length > 0,
    ),
  );

  /**
   * @param {ConnectionType} type - connection type
   * @returns {string} - its display label
   */
  function typeLabel(type: ConnectionType): string {
    return CONNECTIONS.find((option) => option.value === type)?.label ?? '';
  }

  /**
   * Adds or removes a model; the menu closes once every slot is taken
   * @param {string} key - model key
   */
  function toggle(key: string): void {
    chosen = chosen.includes(key) ? chosen.filter((item) => item !== key) : [...chosen, key];
    if (picks.length >= MAX_MODELS) open = false;
  }
</script>

<svelte:window onkeydown={(event) => event.key === 'Escape' && (open = false)} />

<div class="picker">
  <div class="slots">
    {#each picks as model (model.key)}
      <div class="slot filled" style:--model={model.color}>
        <ModelName name={model.label} color={model.color} />
        <span class="via">{typeLabel(model.type)}</span>
        <button class="remove" type="button" aria-label="Retirer {model.label}" onclick={() => toggle(model.key)}><Icon name="cross" size={14} /></button>
      </div>
    {/each}
    {#each { length: MAX_MODELS - picks.length } as _, index (index)}
      {#if index === 0}
        <button class="slot empty" type="button" aria-expanded={open} onclick={() => (open = !open)}><Icon name="plus" size={16} /> Ajouter</button>
      {:else}
        <div class="slot empty" aria-hidden="true"></div>
      {/if}
    {/each}
  </div>

  {#if open}
    <div class="menu" role="group" aria-label="Modèles disponibles">
      {#each columns as column (column.type)}
        <section style:flex-grow={column.models.length}>
          <h3>{column.title}{#if column.type === 'local'}<span class="note">&ensp;·&ensp;{MAX_LOCAL} au plus</span>{/if}</h3>
          <div class="options">
            {#each column.models as model (model.key)}
              {@const on = chosen.includes(model.key)}
              <button class="option" type="button" aria-pressed={on} disabled={!on && pickBlock(types, model.type) !== ''} onclick={() => toggle(model.key)}>
                <span class="tick"><Icon name="check" size={14} /></span>
                <ModelName name={model.label} color={model.color} />
              </button>
            {/each}
          </div>
        </section>
      {/each}
    </div>
  {/if}
</div>

<style>
  .picker {
    display: grid;
    gap: var(--s2);
  }

  .slots {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--s2);
  }

  .slot {
    display: grid;
    align-content: center;
    min-height: 56px;
    padding: var(--s2) var(--s3);
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
  }

  /* The color bar matches the model's answer card and dot. */
  .filled {
    position: relative;
    gap: 2px;
    padding-right: 36px;
    background: var(--panel);
    box-shadow: inset 3px 0 0 var(--model);
  }

  .via {
    color: var(--ink-3);
    font-size: var(--text-sm);
  }

  .remove {
    position: absolute;
    top: 50%;
    right: var(--s1);
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border: 0;
    border-radius: var(--radius);
    background: none;
    color: var(--ink-3);
    cursor: pointer;
    transform: translateY(-50%);
  }

  .remove:hover {
    color: var(--ink);
  }

  .empty {
    border-style: dashed;
    border-color: var(--axis);
    opacity: 0.5;
  }

  button.empty {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--s2);
    background: none;
    color: var(--ink-2);
    font: inherit;
    opacity: 1;
    cursor: pointer;
  }

  button.empty:hover,
  button.empty[aria-expanded='true'] {
    border-color: var(--accent);
    color: var(--ink);
  }

  /* Each type widens with its model count, a long list flows into several columns. */
  .menu {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s4) var(--s5);
    max-height: 50vh;
    padding: var(--s3);
    overflow-y: auto;
    border: 1px solid var(--hairline);
    border-radius: var(--radius);
    background: var(--raised);
  }

  section {
    flex-basis: 240px;
  }

  .options {
    columns: 240px;
    column-gap: var(--s2);
  }

  h3 {
    margin: 0 0 var(--s2);
    padding-inline: var(--s2);
    color: var(--ink-2);
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .note {
    color: var(--ink-3);
    font-weight: 400;
  }

  .option {
    display: flex;
    align-items: center;
    gap: var(--s2);
    width: 100%;
    min-height: 36px;
    padding: var(--s1) var(--s2);
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    color: var(--ink);
    text-align: left;
    cursor: pointer;
    break-inside: avoid;
  }

  .option:hover:not(:disabled) {
    background: color-mix(in srgb, var(--ink) 6%, transparent);
  }

  .option[aria-pressed='true'] {
    background: color-mix(in srgb, var(--accent) 14%, transparent);
  }

  .option:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .tick {
    display: grid;
    color: var(--accent);
    visibility: hidden;
  }

  .option[aria-pressed='true'] .tick {
    visibility: visible;
  }

  @media (max-width: 760px) {
    .slots {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
</style>
