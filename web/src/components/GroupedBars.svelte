<script lang="ts" module>
  export interface BarGroup {
    label: string;
    bars: { model: string; value: number; text: string; color: string; detail: string }[];
  }
</script>

<script lang="ts">
  let { title, groups, max }: { title: string; groups: BarGroup[]; max: number } = $props();
</script>

<figure class="panel chart">
  <figcaption><h3>{title}</h3></figcaption>
  <div class="groups">
    {#each groups as group (group.label)}
      <div class="group">
        <div class="group-label">{group.label}</div>
        <div class="bars">
          {#each group.bars as bar (bar.model)}
            <!-- svelte-ignore a11y_no_noninteractive_tabindex: keyboard focus shows the same tooltip as hover -->
            <div class="row" tabindex="0" role="img" aria-label="{group.label}, {bar.model} : {bar.text}">
              <span class="bar" style:width="{max > 0 ? (100 * bar.value) / max : 0}%" style:background={bar.color}></span>
              <span class="value">{bar.text}</span>
              <span class="tip" role="presentation"><strong>{bar.model}</strong><br />{bar.detail}</span>
            </div>
          {/each}
        </div>
      </div>
    {/each}
  </div>
</figure>

<style>
  .chart {
    display: grid;
    gap: var(--s4);
    margin: 0;
    min-width: 0;
  }

  .groups {
    display: grid;
    gap: var(--s3);
  }

  .group {
    display: grid;
    grid-template-columns: 9.5rem minmax(0, 1fr);
    align-items: center;
    gap: var(--s3);
  }

  .group-label {
    color: var(--ink-2);
    font-size: var(--text-sm);
  }

  .bars {
    display: grid;
    gap: 2px;
    padding-left: 1px;
    border-left: 1px solid var(--axis);
  }

  .row {
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--s2);
    min-height: 24px;
    border-radius: 2px;
  }

  .bar {
    display: block;
    /* The value keeps its room after the bar, even at the maximum. */
    max-width: calc(100% - 4rem);
    height: 18px;
    min-width: 2px;
    border-radius: 0 4px 4px 0;
    transition: width 0.5s var(--ease-out);
  }

  .value {
    flex: none;
    font-size: var(--text-sm);
    font-variant-numeric: tabular-nums;
  }

  .tip {
    position: absolute;
    bottom: calc(100% + var(--s1));
    left: var(--s4);
    z-index: 2;
    display: none;
    width: max-content;
    max-width: 18rem;
    padding: var(--s2) var(--s3);
    border: 1px solid var(--axis);
    border-radius: var(--radius-sm);
    background: var(--raised);
    box-shadow: var(--shadow-float);
    font-size: var(--text-sm);
    overflow-wrap: anywhere;
    pointer-events: none;
  }

  .row:hover .tip,
  .row:focus-visible .tip {
    display: block;
  }

  @media (max-width: 760px) {
    .group {
      grid-template-columns: minmax(0, 1fr);
      gap: var(--s1);
    }
  }
</style>
