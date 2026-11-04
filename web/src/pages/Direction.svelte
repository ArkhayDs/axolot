<script lang="ts">
  import { onMount } from 'svelte';

  import CampaignPicker from '../components/CampaignPicker.svelte';
  import EvalMatrix from '../components/EvalMatrix.svelte';
  import Icon from '../components/Icon.svelte';
  import Notice from '../components/Notice.svelte';
  import { api, errorMessage } from '../lib/api';
  import type { CasesResponse, Hardware } from '../lib/api';
  import { campaignResults, picked } from '../lib/campaigns.svelte';
  import { when } from '../lib/format';

  let hardware = $state<Hardware | null>(null);
  let cases = $state<CasesResponse | null>(null);
  let loadError = $state('');

  const results = campaignResults(() => picked.id);
  const models = $derived([...new Set((results.detail?.stats ?? []).map((stat) => stat.model))].sort());
  const shownMachine = $derived(results.detail?.campaign.hardware ?? hardware);
  const questions = $derived((cases?.cases ?? []).reduce((total, item) => total + item.items.length, 0));

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
    <div class="matrix">
      <EvalMatrix stats={results.detail.stats} verdicts={results.detail.verdicts} {models} />
      <p class="muted caption">Plus une case est rouge, plus le modèle a perdu de points. Recommandé : le plus rapide à moins de 10 points du meilleur score.</p>
    </div>
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
  .matrix {
    display: grid;
    gap: var(--s2);
    min-width: 0;
  }

  .caption {
    font-size: var(--text-sm);
  }

  .how {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: var(--s3) var(--s4);
  }

  .cta {
    align-self: center;
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

  @media (max-width: 760px) {
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
