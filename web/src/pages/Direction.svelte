<script lang="ts">
  import { onMount } from 'svelte';

  import CampaignPicker from '../components/CampaignPicker.svelte';
  import Icon from '../components/Icon.svelte';
  import type { IconName } from '../components/Icon.svelte';
  import ModelName from '../components/ModelName.svelte';
  import Notice from '../components/Notice.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { CasesResponse, Hardware, Pick } from '../lib/api';
  import { campaignResults, picked } from '../lib/campaigns.svelte';
  import { categoryLabel, duration, score, seriesColor, when } from '../lib/format';

  let hardware = $state<Hardware | null>(null);
  let cases = $state<CasesResponse | null>(null);
  let loadError = $state('');

  const results = campaignResults(() => picked.id);
  const models = $derived([...new Set((results.detail?.stats ?? []).map((stat) => stat.model))].sort());
  const shownMachine = $derived(results.detail?.campaign.hardware ?? hardware);
  const questions = $derived((cases?.cases ?? []).reduce((total, item) => total + item.items.length, 0));

  const COLUMNS: { key: 'quality' | 'speed' | 'compromise'; label: string; icon: IconName }[] = [
    { key: 'quality', label: 'Qualité', icon: 'star' },
    { key: 'speed', label: 'Vitesse', icon: 'bolt' },
    { key: 'compromise', label: 'Compromis', icon: 'balance' },
  ];

  /**
   * @param {Pick} pick - winning model
   * @param {string} key - verdict column
   * @returns {string} - figure justifying the win
   */
  function figure(pick: Pick, key: string): string {
    if (key === 'quality') return `${score(pick.score)}/100`;
    if (key === 'speed') return duration(pick.p50_total_ms);
    return `${score(pick.score)}/100, ${duration(pick.p50_total_ms)}`;
  }

  onMount(async () => {
    try {
      const [status, loaded] = await Promise.all([api.status(), api.cases()]);
      hardware = status.hardware;
      cases = loaded;
    } catch (error) {
      loadError = errorMessage(error);
    }
  });
</script>

<div class="page">
  <div class="page-head">
    <div>
      <h1>Quel modèle pour quelle tâche ?</h1>
      {#if results.detail}
        <p>
          {results.detail.campaign.done_items} réponses notées{shownMachine ? ` sur ${shownMachine.label}` : ''}, {when(results.detail.campaign.created_at)}
        </p>
      {/if}
    </div>
    <CampaignPicker bind:value={picked.id} campaigns={picked.list} />
  </div>

  <Notice message={loadError || results.error || picked.error} />

  {#if results.detail && results.detail.verdicts.length > 0}
    <div class="table-wrap">
      <table class="verdicts">
        <thead>
          <tr>
            <th scope="col">Tâche</th>
            {#each COLUMNS as column (column.key)}
              <th scope="col"><span class="head-cell"><Icon name={column.icon} /> {column.label}</span></th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each results.detail.verdicts as verdict (verdict.category)}
            <tr class:overall={verdict.category === ''}>
              <th scope="row">{categoryLabel(verdict.category)}</th>
              {#each COLUMNS as column (column.key)}
                {@const pick = verdict[column.key]}
                <td data-label={column.label}>
                  {#if pick}
                    <span class="winner">
                      <ModelName name={pick.model} color={seriesColor(pick.model, models)} />
                      <span class="figure">{figure(pick, column.key)}</span>
                    </span>
                  {:else}
                    <span class="muted">-</span>
                  {/if}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <p class="muted caption">Temps médian par réponse. Compromis : le plus rapide à moins de 10 points du meilleur score.</p>
  {:else if !results.error && !picked.error}
    <div class="empty">
      <h2>Aucun résultat pour l'instant</h2>
      <p>Une première campagne remplit ce tableau.</p>
      <a class="btn btn-primary" href="#/labo"><Icon name="play" /> Lancer une campagne</a>
    </div>
  {/if}

  <section class="section" aria-labelledby="how">
    <h2 id="how">Comment ça marche</h2>
    <div class="how panel">
      <a class="step" href="#/cas">
        <Icon name="list" size={24} />
        <span><strong>Des cas à réponse connue</strong><span class="muted">{cases ? `${cases.cases.length} cas, ${questions} questions` : 'Code, documents, tri...'}</span></span>
      </a>
      <span class="link"><Icon name="arrow" /></span>
      <a class="step" href="#/reglages">
        <Icon name="cpu" size={24} />
        <span><strong>Les modèles de cette machine</strong><span class="muted">{hardware ? hardware.label : 'Un à la fois'}</span></span>
      </a>
      <span class="link"><Icon name="arrow" /></span>
      <a class="step" href="#/resultats">
        <Icon name="grade" size={24} />
        <span><strong>Une note sans IA</strong><span class="muted">Vérifications automatiques, sources ouvrables</span></span>
      </a>
      <a class="btn btn-primary cta" href="#/labo"><Icon name="flask" /> Voir en direct</a>
    </div>
  </section>
</div>

<style>
  .verdicts tbody th {
    color: var(--ink);
    font-size: var(--text-md);
    font-weight: 600;
    white-space: nowrap;
  }

  .overall {
    background: var(--raised);
  }

  .head-cell {
    display: inline-flex;
    align-items: center;
    gap: var(--s2);
  }

  .verdicts td {
    min-width: 13rem;
    padding-block: var(--s2);
  }

  .winner {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0 var(--s3);
  }

  .figure {
    font-weight: 600;
  }

  .caption {
    margin-top: calc(-1 * var(--s4));
    font-size: var(--text-sm);
  }

  .how {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--s3) var(--s4);
  }

  .step {
    display: flex;
    flex: 1 1 13rem;
    align-items: flex-start;
    gap: var(--s3);
    border-radius: var(--radius-sm);
    text-decoration: none;
  }

  .step:hover strong {
    color: var(--accent);
  }

  .step strong {
    display: block;
    font-weight: 600;
    transition: color 0.15s;
  }

  .step span span {
    display: block;
    font-size: var(--text-sm);
  }

  .link {
    color: var(--axis);
  }

  /* A phone reads the table as one block per task: every cell repeats its column name. */
  @media (max-width: 760px) {
    .verdicts thead {
      display: none;
    }

    .verdicts tr,
    .verdicts tbody th,
    .verdicts td {
      display: block;
    }

    .verdicts tbody th {
      padding-bottom: 0;
      border-bottom: 0;
    }

    .verdicts td {
      min-width: 0;
      padding-block: var(--s2);
      border-bottom: 0;
    }

    .verdicts td::before {
      content: attr(data-label);
      display: block;
      color: var(--ink-3);
      font-size: var(--text-sm);
    }

    .verdicts tr {
      padding-bottom: var(--s2);
      border-bottom: 1px solid var(--hairline);
    }

    .how {
      align-items: stretch;
      flex-direction: column;
    }

    .step {
      flex: none;
    }

    .link {
      display: none;
    }
  }
</style>
