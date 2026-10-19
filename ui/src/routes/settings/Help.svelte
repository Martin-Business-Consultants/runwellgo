<script>
  // Help › Overview: how Runwell works, in this install's own words, in
  // tabs (?tab=): what it is, the hierarchy, how work flows (and
  // commitments), how it's different, and finding your way around.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import ArrowRight from "@lucide/svelte/icons/arrow-right";
  import ArrowDown from "@lucide/svelte/icons/arrow-down";
  import { get } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { pageTabs } from "$lib/page-tabs.svelte.js";

  let help = $state(null);
  const tabs = pageTabs(["what", "hierarchy", "flow", "different", "around"]);
  get("/settings/help").then((h) => (help = h)).catch(() => {});

  const t = (k, n = 1) => session.term(k, n);
  const lc = (k, n = 1) => session.term(k, n).toLowerCase();
  const cap = (s) => s.charAt(0).toUpperCase() + s.slice(1);
  const labels = $derived(Object.entries(session.me?.terms?.labels || {}).filter(([, l]) => l.enabled !== false));
  const sentence = (items, word) => items.length < 2 ? items.join("") : items.slice(0, -1).join(", ") + ` ${word} ` + items.at(-1);
  const types = $derived(sentence(labels.map(([, l]) => (/^[aeiou]/i.test(l.one) ? "an " : "a ") + l.one.toLowerCase()), "or"));
  const refs = $derived(sentence(labels.map(([, l]) => `a ${l.one.toLowerCase()} (${l.prefix}-3)`), "or"));
</script>

<SettingsLayout title="How Runwell works">
  <Tabs.Root value={tabs.value} onValueChange={tabs.set} class="gap-6">
    <Tabs.List aria-label="How Runwell works">
      <Tabs.Trigger value="what">What it is</Tabs.Trigger>
      <Tabs.Trigger value="hierarchy">The hierarchy</Tabs.Trigger>
      <Tabs.Trigger value="flow">How work flows</Tabs.Trigger>
      <Tabs.Trigger value="different">How it’s different</Tabs.Trigger>
      <Tabs.Trigger value="around">Finding your way</Tabs.Trigger>
    </Tabs.List>
    <Tabs.Content value="what">
      <Card.Root>
        <Card.Header>
          <Card.Title>What it is</Card.Title>
          <Card.Description>This is Runwell {session.version}.{#if session.can("manage_settings")}{" "}<a href="/settings/updates" class="underline underline-offset-4">Check for updates</a>{/if}</Card.Description>
        </Card.Header>
        <Card.Content class="flex flex-col gap-4 text-sm leading-relaxed">
          <p>Runwell is the core of a business’s project management, and works as well for anything you run: a home, a wedding, a trip. It answers five questions and nothing else:</p>
          <figure class="flex flex-col gap-3">
            <div class="grid items-stretch gap-3 md:grid-cols-[1fr_auto_1fr_auto_1fr]">
              <div class="rounded-lg border p-3"><Badge variant="secondary">1</Badge> <strong>Who we work for</strong><p class="text-muted-foreground text-xs">and who can say yes</p><p class="mt-2">{t("client")} · Contacts <span class="text-muted-foreground">(some can approve)</span></p></div>
              <div class="text-muted-foreground flex items-center justify-center text-xs"><span class="hidden md:inline">agree to</span><ArrowRight class="hidden size-4 md:inline" /><ArrowDown class="size-4 md:hidden" /></div>
              <div class="rounded-lg border p-3"><Badge variant="secondary">2</Badge> <strong>What we agreed</strong><p class="text-muted-foreground text-xs">sent once, frozen for good</p><p class="mt-2">{t("engagement")} <span class="text-muted-foreground">({labels.map(([, l]) => l.prefix + "-3").join(", ")})</span> · Agreement versions · {t("scope_item", 2)} <span class="text-muted-foreground">with prices</span></p></div>
              <div class="text-muted-foreground flex items-center justify-center text-xs"><span class="hidden md:inline">approving creates</span><ArrowRight class="hidden size-4 md:inline" /><ArrowDown class="size-4 md:hidden" /></div>
              <div class="rounded-lg border p-3"><Badge variant="secondary">3</Badge> <strong>What is happening</strong><p class="text-muted-foreground text-xs">against what we agreed</p><p class="mt-2">{t("work")} <span class="text-muted-foreground">(planned → done)</span> · {t("commitment", 2)} <span class="text-muted-foreground">(promises with a date, theirs and yours)</span> · Notes and history</p></div>
            </div>
            <div class="grid gap-3 md:grid-cols-2">
              <div class="rounded-lg border p-3"><Badge variant="secondary">4</Badge> <strong>What came in</strong><p class="text-muted-foreground text-xs">from the portal, email or an agent</p><p class="mt-2">Requests <span class="text-muted-foreground">triaged into a new {lc("engagement")}, a {lc("scope_item")}, {lc("work")} or a {lc("commitment")}</span></p></div>
              <div class="rounded-lg border p-3"><Badge variant="secondary">5</Badge> <strong>What needs a person now</strong><p class="text-muted-foreground text-xs">gathered from all of the above, on Home</p>
                <div class="mt-2 flex flex-wrap gap-1">{#each ["Questions for you", "Requests to triage", `Waiting on a ${lc("client")}`, "Drafts to send", "Waiting on them, and what you promised", `Blocked or overdue ${lc("work")}`] as chip (chip)}<Badge variant="outline">{chip}</Badge>{/each}</div>
              </div>
            </div>
            <figcaption class="text-muted-foreground text-xs">{cap(lc("client", 2))} agree to what you send; their approval turns it into {lc("work")}; what they ask for along the way is triaged into the agreement or the {lc("work")}; and Home tells you what’s waiting on you.</figcaption>
          </figure>
          <ol class="list-decimal space-y-1 ps-5">
            <li><strong>Who do we work for, and who can say yes?</strong> {cap(lc("client", 2))} and their contacts.</li>
            <li><strong>What did we agree?</strong> {cap(lc("engagement", 2))} and their agreements.</li>
            <li><strong>What is happening against it?</strong> {t("work")}, {lc("commitment", 2)}, notes and history.</li>
            <li><strong>What came in?</strong> Requests.</li>
            <li><strong>What needs a person now?</strong> The home page.</li>
          </ol>
        </Card.Content>
      </Card.Root>
    </Tabs.Content>

    <Tabs.Content value="hierarchy">
      <Card.Root>
        <Card.Header><Card.Title>The hierarchy</Card.Title></Card.Header>
        <Card.Content class="text-sm leading-relaxed">
          <ul class="flex flex-col gap-1 [&_ul]:mt-1 [&_ul]:border-s [&_ul]:ps-4">
            <li><strong>{t("client")}</strong> <span class="text-muted-foreground">who we work for</span>
              <ul class="flex flex-col gap-1">
                <li><strong>Contacts</strong> <span class="text-muted-foreground">the people there; some <em>can approve</em> agreements</span></li>
                <li><strong>{t("engagement")}</strong> <span class="text-muted-foreground">what we agreed to do: {refs}, fixed scope or recurring</span>
                  <ul class="flex flex-col gap-1">
                    <li><strong>Agreement versions</strong> <span class="text-muted-foreground">the initial agreement, then change orders, revisions or add-ons</span>
                      <ul><li><strong>{t("scope_item", 2)}</strong> <span class="text-muted-foreground">each line the {lc("client")} agrees to, with a price and a staff-only estimate</span>
                        <ul><li><strong>{t("work")}</strong> <span class="text-muted-foreground">approving adds one {lc("work")} item per {lc("scope_item")}; add more under it as you go. Each has an owner, a status and a due date</span></li></ul>
                      </li></ul>
                    </li>
                    <li><strong>Other {lc("work")}</strong> <span class="text-muted-foreground">{cap(lc("work"))} on the {lc("engagement")} that isn’t one {lc("scope_item")}, such as a quick fix or a request made into work</span></li>
                    <li><strong>{cap(lc("commitment", 2))}</strong> <span class="text-muted-foreground">dated promises, yours or the {lc("client")}’s</span></li>
                  </ul>
                </li>
                <li><strong>{cap(lc("commitment", 2))}</strong> <span class="text-muted-foreground">promises that belong to the {lc("client")} rather than one {lc("engagement")}</span></li>
              </ul>
            </li>
            <li><strong>Requests</strong> <span class="text-muted-foreground">asks that come in from a {lc("client")}, the portal, email or a call, before they are anything else</span></li>
          </ul>
          <p class="text-muted-foreground mt-3 text-xs">Notes, documents and history hang off every level: {lc("client", 2)}, {lc("engagement", 2)}, {lc("scope_item", 2)}, {lc("work")} and requests. Each page links up a level in its breadcrumbs.</p>
        </Card.Content>
      </Card.Root>
    </Tabs.Content>

    <Tabs.Content value="flow" class="flex flex-col gap-6">
      <Card.Root>
        <Card.Header><Card.Title>How work flows</Card.Title></Card.Header>
        <Card.Content class="flex flex-col gap-4 text-sm leading-relaxed">
          <div class="flex flex-wrap items-center gap-2 text-xs">
            {#each ["Request or idea", "Draft", "Sent", "Approved", "Closed"] as stage, i (stage)}
              {#if i}<ArrowRight class="text-muted-foreground size-3" />{/if}<Badge variant={stage === "Approved" ? "default" : "outline"}>{stage}</Badge>
            {/each}
            <span class="text-muted-foreground">· changes requested loop back to Draft · approved work goes Planned → In progress → In review → Done</span>
          </div>
          <ol class="list-decimal space-y-1 ps-5">
            <li><strong>A request comes in.</strong> A contact sends it from the portal, or you log it from an email or a call.</li>
            <li><strong>You triage it.</strong> It becomes a new {lc("engagement")}, a {lc("scope_item")} on an open draft, a piece of work or a {lc("commitment")}, or it is dismissed with a reason.</li>
            <li><strong>You draft the agreement.</strong> {cap(lc("scope_item", 2))}, prices and the summary the {lc("client")} will read.</li>
            <li><strong>You send it.</strong> Sending takes a snapshot, fingerprints it and freezes it. Nothing sent ever changes.</li>
            <li><strong>The {lc("client")} decides.</strong> They approve or ask for changes from an emailed link, or you record their decision with evidence such as a forwarded email.</li>
            <li><strong>Approval creates the work.</strong> Every approved {lc("scope_item")} becomes {lc("work")}, tracked on the {t("work")} table or board.</li>
            <li><strong>Work moves through its statuses.</strong> Planned, In progress, In review, Blocked, Done. An AI assistant finishes into In review, and a person checks it and marks it Done.</li>
            <li><strong>Changes are new versions.</strong> A change order (fixed scope) or a revision or add-on (recurring) goes through the same send and approve steps.</li>
            <li><strong>You close the {lc("engagement")}</strong> when it’s done. Its agreement and history stay.</li>
            <li><strong>Your own projects skip the agreement.</strong> Mark the {lc("client")} that is you as internal, and its {lc("engagement", 2)} are Internal from the start: no price, nothing to send or approve, {lc("work")} added straight away.</li>
          </ol>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header><Card.Title>{cap(lc("commitment", 2))}: work versus promises</Card.Title></Card.Header>
        <Card.Content class="flex flex-col gap-3 text-sm leading-relaxed">
          <p>{t("work")} is what you do. A {lc("commitment")} is a promise with a date between you and a {lc("client")}, and it reads as a sentence: <em>Rosa will send the logo · by Friday</em>, or <em>We will share the first draft · by the 14th</em>.</p>
          <div class="grid gap-3 sm:grid-cols-2">
            <div class="rounded-lg border p-3"><strong>Waiting on them</strong><p class="text-muted-foreground">The {lc("client")} or one of their contacts promised you something: content, a decision, access. Your {lc("work")} may be stuck until it arrives.</p></div>
            <div class="rounded-lg border p-3"><strong>You promised</strong><p class="text-muted-foreground">You, or someone on your side, told them they’d have something by a date. Keep it, or say early that it will slip.</p></div>
          </div>
          <p>Add one on a {lc("client")} or {lc("engagement")} page (“[Who] will [what] by [date]”), or triage a request into one. When the date comes it is resolved as done, missed or dropped, and stays as history. Home shows both lanes for the week ahead.</p>
        </Card.Content>
      </Card.Root>
    </Tabs.Content>

    <Tabs.Content value="different">
      <Card.Root>
        <Card.Header><Card.Title>How it’s different</Card.Title></Card.Header>
        <Card.Content class="text-sm leading-relaxed">
          <ul class="list-disc space-y-1 ps-5">
            <li><strong>No invoicing, no timesheets.</strong> {cap(lc("scope_item", 2))} carry the price the {lc("client")} agrees to, but nothing is “billable”: everything is {types}. Plugins add time tracking and invoicing (Settings › Plugins) for teams that want them.</li>
            <li><strong>Agreements are frozen once sent.</strong> Nobody can edit what a {lc("client")} saw. A change is a new version the {lc("client")} approves again.</li>
            <li><strong>State is derived, never typed in.</strong> Each {lc("engagement")} is Draft, Sent, Changes requested, Approved, Internal or Closed because of what happened to its versions, not because someone picked it from a list.</li>
            <li><strong>History is append-only.</strong> Every change of consequence is recorded with who did it and where it came from (the app, the portal, an email). Corrections are new entries.</li>
            <li><strong>{cap(lc("client", 2))} never have passwords.</strong> They sign in to the portal and approve agreements from signed links that expire.</li>
            <li><strong>Internal estimates stay internal.</strong> {cap(lc("client", 2))} see the frozen snapshot they approved, never estimates or estimate notes.</li>
            <li><strong>@mention a teammate</strong> in any note, description or request by typing <kbd>@</kbd>. They get it in Notifications and by email.</li>
            <li><strong>Home is what needs you.</strong> Questions for you, requests to triage, agreements waiting on a {lc("client")}, drafts ready to send, what you’re waiting on them for and what you’ve promised them, and blocked or overdue work.</li>
          </ul>
        </Card.Content>
      </Card.Root>
    </Tabs.Content>

    <Tabs.Content value="around" class="flex flex-col gap-6">
      <Card.Root>
        <Card.Header><Card.Title>Where things are</Card.Title></Card.Header>
        <Card.Content class="text-sm leading-relaxed">
          <ul class="list-disc space-y-1 ps-5">
            <li><strong>Home</strong> (the install’s name at the top of the menu): what needs a person now.</li>
            <li><strong>{t("client", 2)}</strong>, then each one’s contacts, {lc("engagement", 2)}, {lc("commitment", 2)}, notes and history.</li>
            <li><strong>{t("engagement", 2)}</strong>, showing active ones by default (approved or internal, and open). Switch the filter to see drafts, sent, changes requested, internal or closed. Each one holds its agreement, scope, work and commitments.</li>
            <li><strong>{t("work")}</strong>, as a table or a board, across every {lc("engagement")}.</li>
            <li><strong>{cap(lc("commitment", 2))}</strong>: open ones in two lanes, waiting on them and you promised; resolved ones as history.</li>
            <li><strong>Requests</strong>, everything that came in and what it became.</li>
            <li><strong>Search</strong> in the menu (<kbd>s</kbd> or <kbd>⌘K</kbd>) finds {lc("client", 2)}, contacts, {lc("engagement", 2)}, requests, work, {lc("commitment", 2)} and notes.</li>
          </ul>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header><Card.Title>What your {lc("client", 2)} see</Card.Title></Card.Header>
        <Card.Content class="text-sm leading-relaxed">
          <ul class="list-disc space-y-1 ps-5">
            <li><strong>Contacts with portal access</strong> sign in from an emailed link, with no password. They see the agreements they were sent, the {lc("work")} and documents you mark visible to the {lc("client")}, and can send you requests.</li>
            <li><strong>Contacts who can approve</strong> get the approve or request-changes link when you send an agreement.</li>
            <li><strong>Everything else stays internal</strong>: notes, estimates, history, and any {lc("work")} or document not switched to visible.</li>
          </ul>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header><Card.Title>Your AI assistant</Card.Title></Card.Header>
        <Card.Content class="text-sm leading-relaxed">
          Anything you can do here, your own AI assistant (Muse, Grok, Claude, ChatGPT and the like) can do for you, as <strong>{help?.agent_name || "your agent"}</strong>, with your role.
          <a href="/settings/help/mcp" class="underline underline-offset-4">Connecting an MCP</a> sets it up in a minute; <a href="/settings/help/ai" class="underline underline-offset-4">Using with AI</a> has what to ask it.
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header><Card.Title>Keyboard shortcuts</Card.Title></Card.Header>
        <Card.Content class="text-sm">
          <dl class="grid gap-x-6 gap-y-2 sm:grid-cols-[8rem_1fr]">
            <dt><kbd>s</kbd> or <kbd>⌘K</kbd></dt><dd>Search everything</dd>
            <dt><kbd>⌘B</kbd></dt><dd>Fold the menu to icons, or open it again</dd>
            <dt><kbd>Esc</kbd></dt><dd>Close a dialog, or leave a form</dd>
          </dl>
        </Card.Content>
      </Card.Root>
    </Tabs.Content>
  </Tabs.Root>
</SettingsLayout>
