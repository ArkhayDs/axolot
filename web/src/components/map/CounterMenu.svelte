<script lang="ts">
  import { onMount } from 'svelte';

  import type { Findable } from '../../lib/map/findable';

  let {
    title,
    entries,
    isSelected,
    onpick,
    onclose,
  }: { title: string; entries: Findable[]; isSelected: (entry: Findable) => boolean; onpick: (entry: Findable) => void; onclose: (refocus: boolean) => void } = $props();

  let menu: HTMLDivElement;

  /** @param {KeyboardEvent} event - arrows walk the list, Escape closes */
  function onKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.stopPropagation();
      onclose(true);
      return;
    }
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;
    event.preventDefault();
    const boxes = [...menu.querySelectorAll('input')];
    const at = boxes.indexOf(document.activeElement as HTMLInputElement);
    boxes[(at + (event.key === 'ArrowDown' ? 1 : -1) + boxes.length) % boxes.length]?.focus();
  }

  onMount(() => {
    menu.querySelector('input')?.focus();
    const outside = (event: PointerEvent): void => {
      const target = event.target as HTMLElement;
      // The counter's own button toggles the list: closing here would reopen it on the click that follows.
      if (!menu.contains(target) && !target.closest('[data-counter]')) onclose(false);
    };
    document.addEventListener('pointerdown', outside);
    return () => document.removeEventListener('pointerdown', outside);
  });
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="menu" role="group" aria-label={title} bind:this={menu} onkeydown={onKeydown}>
  {#each entries as entry (`${entry.kind}:${entry.key}`)}
    <label>
      <input type="checkbox" checked={isSelected(entry)} onchange={() => onpick(entry)} />
      <span class="name" title={entry.label}>{entry.label}</span>
      <span class="note">{entry.note}</span>
    </label>
  {/each}
</div>

<style>
  .menu {
    position: absolute;
    bottom: calc(100% + var(--s3));
    left: 0;
    z-index: 2;
    display: grid;
    width: 22rem;
    max-height: 50vh;
    overflow-y: auto;
    padding: var(--s1);
    border: 1px solid var(--axis);
    border-radius: var(--radius);
    background: var(--panel);
    box-shadow: var(--shadow-float);
    scrollbar-color: var(--axis) transparent;
  }

  label {
    display: flex;
    align-items: center;
    gap: var(--s2);
    padding: var(--s1) var(--s2);
    border-radius: var(--radius-sm);
    color: var(--ink);
    font-size: var(--text-sm);
    cursor: pointer;
  }

  label:hover,
  label:focus-within {
    background: var(--raised);
  }

  input {
    flex: none;
    margin: 0;
    accent-color: var(--accent);
  }

  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .note {
    flex: none;
    color: var(--ink-2);
    font-variant-numeric: tabular-nums;
  }
</style>
