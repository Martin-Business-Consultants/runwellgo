<script>
  // The audit sheet on a website's page (the Rails AI audit), two-thirds of
  // the window wide: the domain to audit, then, once the audit has run in
  // the background (the sheet looks again every few seconds meanwhile), what
  // was found about it from outside: DNS, the registry, the home page,
  // robots.txt and the sitemap. The AI's recommendation per step needs
  // Runwell's AI plugin, which this install doesn't have, so an audit here
  // gathers the facts; recommendations an imported install made are listed
  // on the left, the one picked on the right, to approve as it is, change
  // and approve, or reject.
  import * as Sheet from "$lib/components/ui/sheet/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Textarea } from "$lib/components/ui/textarea/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Search from "@lucide/svelte/icons/search";
  import Check from "@lucide/svelte/icons/check";
  import X from "@lucide/svelte/icons/x";
  import LoaderCircle from "@lucide/svelte/icons/loader-circle";
  import StateDot from "./StateDot.svelte";
  import { get, post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { date, timeAgo, humanize } from "$lib/format.js";
  import { wc } from "./wc.js";

  let { open = $bindable(false), site, ondone } = $props();
  let data = $state(null), domain = $state(""), busy = $state(false), picked = $state(null);
  let edit = $state({ value: "", note: "" });

  async function load() {
    try { show(await get(wc(`/websites/${site.id}/scans`))); } catch (e) { toastError(e); }
  }
  function show(r) {
    data = r;
    domain = r.scan?.domain || r.host || "";
    const recs = r.scan?.recommendations || [];
    pick((recs.find((x) => x.id === picked?.id) && picked.state === "pending" ? picked : null) || recs.find((x) => x.state === "pending") || recs[0] || null);
  }
  function pick(rec) {
    picked = rec;
    edit = { value: rec?.value || "", note: rec?.note || "" };
  }
  $effect(() => { if (open) load(); });
  // While an audit runs, look again every three seconds.
  $effect(() => {
    if (!open || !data?.scan?.working) return;
    const t = setTimeout(load, 3000);
    return () => clearTimeout(t);
  });

  async function run(e) {
    e.preventDefault();
    busy = true;
    try { const r = await post(wc(`/websites/${site.id}/scans`), { scan: { domain } }); toast(r.message); show(r); } catch (err) { toastError(err); }
    busy = false;
  }
  async function decide(kind) {
    try {
      const r = await post(wc(`/websites/${site.id}/recommendations/${picked.id}/${kind}`), kind === "approval" ? { recommendation: edit } : {});
      toast(r.message);
      show(r);
    } catch (e) { toastError(e); }
  }
  // Closing it draws the page again: an approval recorded a step.
  $effect(() => { if (!open && data) ondone?.(); });

  const recs = $derived(data?.scan?.recommendations || []);
  const pending = $derived(recs.filter((r) => r.state === "pending").length);
  const facts = $derived(data?.scan?.facts || null);
  const who = (id) => (session.me.team || []).find((p) => p.id === id)?.name || "someone";
  const tones = { high: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400", medium: "bg-amber-500/15 text-amber-700 dark:text-amber-400", low: "bg-muted text-muted-foreground" };
  const list = (v) => (Array.isArray(v) && v.length ? v.join(", ") : "None found");
</script>

<Sheet.Root bind:open>
  <Sheet.Content side="right" class="w-full overflow-y-auto data-[side=right]:sm:max-w-[66vw]">
    <Sheet.Header><Sheet.Title>Audit · {site.name}</Sheet.Title></Sheet.Header>
    <div class="flex flex-col gap-5 px-4 pb-6">
      {#if !data}
        <Skeleton class="h-32" />
      {:else}
        {#if data.can_manage}
          <form onsubmit={run} class="grid grid-cols-[minmax(0,1fr)_auto] items-end gap-3">
            <Field.Field>
              <Field.Label for="audit-domain">Domain</Field.Label>
              <Input id="audit-domain" bind:value={domain} required autocomplete="off" placeholder="example.com" />
            </Field.Field>
            <Button type="submit" disabled={busy || data.scan?.working}>{#if busy}<LoaderCircle class="animate-spin" />{:else}<Search />{/if}{data.scan ? "Audit again" : "Run the audit"}</Button>
          </form>
        {/if}
        <Alert.Root>
          <Alert.Description>Recommendations for each step need Runwell’s AI plugin, which this install doesn’t have: the audit finds out what it can about the domain and shows it here, and nothing is recorded on the checklist.</Alert.Description>
        </Alert.Root>

        {#if !data.scan}
          <div class="text-muted-foreground flex max-w-prose flex-col gap-2 text-sm">
            <p>The audit finds out what it can about the domain from outside, without logging in anywhere: who it’s registered with and when it renews, who hosts its DNS and email, SPF and DMARC, Google and Bing verifications, how its addresses answer, the Analytics and Tag Manager IDs on its pages, where its forms post, and its sitemap.</p>
          </div>
        {:else if data.scan.working}
          <div class="flex items-center gap-2 text-sm"><LoaderCircle class="size-4 animate-spin" /><span>Auditing <strong>{data.scan.domain}</strong>: reading its DNS, registry record, pages and sitemap. A minute or so.</span></div>
        {:else if data.scan.state === "failed"}
          <Alert.Root variant="destructive"><Alert.Description>The audit of {data.scan.domain} didn’t finish: {data.scan.error || "it stopped answering"}.</Alert.Description></Alert.Root>
        {:else}
          <p class="text-muted-foreground text-xs">
            {data.scan.domain}, audited {timeAgo(data.scan.finished_at || data.scan.started_at)} by {who(data.scan.user_id)} ·
            {recs.length === 1 ? "1 recommendation" : `${recs.length} recommendations`}, {pending} to review
          </p>

          {#if recs.length}
            <div class="grid items-start gap-6 md:grid-cols-2">
              <nav class="flex flex-col gap-0.5" aria-label="Recommendations">
                {#each recs as r (r.id)}
                  <button type="button" onclick={() => pick(r)} aria-current={picked?.id === r.id ? "page" : undefined}
                    class="hover:bg-muted flex items-center gap-2 rounded-md px-2 py-1.5 text-start text-sm aria-[current=page]:bg-muted">
                    <StateDot state={r.state} title={humanize(r.state)} />
                    <span class="flex-1 truncate {r.state === 'pending' ? '' : 'text-muted-foreground'}">{r.item?.title}</span>
                    <span class="text-muted-foreground text-xs">{r.state === "pending" ? r.confidence || "" : r.state}</span>
                  </button>
                {/each}
              </nav>
              {#if picked}
                <article class="flex flex-col gap-4" aria-label={picked.item?.title}>
                  <header class="flex flex-col gap-1">
                    <p class="text-muted-foreground text-xs">{picked.item?.group}</p>
                    <div class="flex items-start justify-between gap-3">
                      <h3 class="font-semibold">{picked.item?.title}</h3>
                      {#if picked.confidence}<Badge variant="outline" class="border-transparent {tones[picked.confidence]}">{humanize(picked.confidence)} confidence</Badge>{/if}
                    </div>
                  </header>
                  <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
                    <dt class="text-muted-foreground">Recorded now</dt><dd class={picked.item?.value ? "" : "text-muted-foreground"}>{picked.item?.value || "Nothing"} ({picked.item?.state_label?.toLowerCase()})</dd>
                    {#if picked.evidence}<dt class="text-muted-foreground">Found in</dt><dd>{picked.evidence}</dd>{/if}
                    {#if picked.attach_file}<dt class="text-muted-foreground">File</dt><dd>{picked.attach_file}, kept on the step when approved</dd>{/if}
                  </dl>
                  {#if picked.state === "pending"}
                    <Field.Group class="gap-3">
                      <Field.Field>
                        <Field.Label for="rec-value">Recommended value</Field.Label>
                        <Textarea id="rec-value" bind:value={edit.value} rows={3} required />
                        <Field.Description>Change it if you know better. Approving records it on the step as yours.</Field.Description>
                      </Field.Field>
                      <Field.Field>
                        <Field.Label for="rec-note">Note</Field.Label>
                        <Textarea id="rec-note" bind:value={edit.note} rows={2} placeholder="Anything still to do" />
                      </Field.Field>
                      <div class="flex gap-2">
                        <Button onclick={() => decide("approval")}><Check />Approve</Button>
                        <Button variant="outline" onclick={() => decide("rejection")}><X />Reject</Button>
                      </div>
                    </Field.Group>
                  {:else}
                    <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
                      <dt class="text-muted-foreground">Recommended</dt><dd class="whitespace-pre-line">{picked.value}</dd>
                      {#if picked.note}<dt class="text-muted-foreground">Note</dt><dd class="whitespace-pre-line">{picked.note}</dd>{/if}
                      <dt class="text-muted-foreground">{picked.state === "approved" ? "Approved by" : "Rejected by"}</dt><dd>{picked.decided_by_name}, {date(picked.decided_at)}</dd>
                    </dl>
                  {/if}
                </article>
              {/if}
            </div>
          {/if}

          {#if facts}
            <div class="grid gap-4 sm:grid-cols-2">
              <div class="flex flex-col gap-2 rounded-lg border p-3 text-sm">
                <strong>DNS</strong>
                {#if facts.dns?.error}<p class="text-muted-foreground">{facts.dns.error}</p>{:else if facts.dns}
                  <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
                    <dt class="text-muted-foreground">Nameservers</dt><dd class="break-words">{list(facts.dns.nameservers)}</dd>
                    <dt class="text-muted-foreground">Mail (MX)</dt><dd class="break-words">{list(facts.dns.mx)}</dd>
                    <dt class="text-muted-foreground">SPF</dt><dd class="break-all">{list(facts.dns.spf)}</dd>
                    <dt class="text-muted-foreground">DMARC</dt><dd class="break-all">{list(facts.dns.dmarc)}</dd>
                    <dt class="text-muted-foreground">Verifications</dt><dd class="break-all">{list(facts.dns.verifications)}</dd>
                    <dt class="text-muted-foreground">Addresses</dt><dd class="break-words">{list([...(facts.dns.a || []), ...(facts.dns.aaaa || [])])}</dd>
                    <dt class="text-muted-foreground">www</dt><dd class="break-words">{list([...(facts.dns.www_cname || []), ...(facts.dns.www_a || [])])}</dd>
                  </dl>
                {/if}
              </div>
              <div class="flex flex-col gap-2 rounded-lg border p-3 text-sm">
                <strong>Registration</strong>
                {#if facts.registration?.error}<p class="text-muted-foreground">{facts.registration.error}</p>{:else if facts.registration}
                  <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
                    <dt class="text-muted-foreground">Registrar</dt><dd>{facts.registration.registrar || "Not given"}</dd>
                    <dt class="text-muted-foreground">Registered</dt><dd>{facts.registration.registered ? date(facts.registration.registered) : "Not given"}</dd>
                    <dt class="text-muted-foreground">Renews</dt><dd>{facts.registration.expires ? date(facts.registration.expires) : "Not given"}</dd>
                    <dt class="text-muted-foreground">Status</dt><dd class="break-words">{list(facts.registration.status)}</dd>
                  </dl>
                {/if}
              </div>
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <div class="flex flex-col gap-2 rounded-lg border p-3 text-sm">
                <strong>Home page</strong>
                {#if facts.home_page?.error}<p class="text-muted-foreground">{facts.home_page.error}</p>{:else if facts.home_page}
                  <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
                    <dt class="text-muted-foreground">Title</dt><dd class="break-words">{facts.home_page.title || "None"}</dd>
                    <dt class="text-muted-foreground">Platform</dt><dd>{list([...(facts.home_page.generator ? [facts.home_page.generator] : []), ...(facts.home_page.platform_hints || [])])}</dd>
                    <dt class="text-muted-foreground">Analytics</dt><dd class="break-words">{list([...(facts.home_page.analytics_ids || []), ...(facts.home_page.universal_analytics_ids || [])])}</dd>
                    <dt class="text-muted-foreground">Tag Manager</dt><dd>{list(facts.home_page.tag_manager_ids)}</dd>
                    <dt class="text-muted-foreground">Forms</dt><dd class="break-words">{facts.home_page.forms}{facts.home_page.form_posts_to?.length ? `, posting to ${facts.home_page.form_posts_to.join(", ")}` : ""}</dd>
                  </dl>
                {/if}
              </div>
              <div class="flex flex-col gap-2 rounded-lg border p-3 text-sm">
                <strong>Addresses, robots.txt and sitemap</strong>
                <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
                  {#if facts.addresses && !facts.addresses.error}
                    {#each Object.entries(facts.addresses) as [u, a] (u)}
                      <dt class="text-muted-foreground">{u.replace(/\/$/, "")}</dt><dd class="break-all">{a.error || `${a.status}${a.redirects?.length ? ` → ${a.ends_at}` : ""}`}</dd>
                    {/each}
                  {/if}
                  <dt class="text-muted-foreground">robots.txt</dt><dd class="break-words">{facts.robots_txt?.error || (facts.robots_txt?.disallows_everything ? "Shuts every crawler out" : "Lets crawlers in")}</dd>
                  <dt class="text-muted-foreground">Sitemap</dt><dd class="break-all">{facts.sitemap?.error || `${facts.sitemap?.url}: ${facts.sitemap?.entries} entries, kept as a file`}</dd>
                </dl>
              </div>
            </div>
            <details class="text-sm">
              <summary class="text-muted-foreground cursor-pointer">What was found about {data.scan.domain}</summary>
              <pre class="bg-muted mt-2 max-h-96 overflow-auto rounded-lg p-3 text-xs">{JSON.stringify(facts, null, 2)}</pre>
            </details>
          {/if}
        {/if}
      {/if}
    </div>
  </Sheet.Content>
</Sheet.Root>
