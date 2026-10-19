<script>
  // Help › Using with AI: what to ask an assistant once it's connected.
  // How to talk to it on top; then tabs (?tab=): the ready-made starts,
  // what to ask by what you do, on a schedule, and staying in control.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import { get } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { pageTabs } from "$lib/page-tabs.svelte.js";

  let help = $state(null);
  const tabs = pageTabs(["starts", "projects", "work", "schedule", "control"]);
  get("/settings/help").then((h) => (help = h)).catch(() => {});
  const agent = $derived(help?.agent_name || "your agent");
  const client = $derived(session.term("client").toLowerCase());
  const clients = $derived(session.term("client", 2).toLowerCase());
  const engagement = $derived(session.term("engagement").toLowerCase());
  const engagements = $derived(session.term("engagement", 2).toLowerCase());
  const work = $derived(session.term("work").toLowerCase());
  const cap = (s) => s.charAt(0).toUpperCase() + s.slice(1);
  // The MCP prompts Runwell offers staff assistants.
  const prompts = [
    ["Plan my day", "What needs me today, and what to do first."],
    ["Standup", "What changed across the team since yesterday, by engagement."],
    ["Triage requests", "Go through open requests and propose what each should become."],
    ["Weekly client update", "Draft this week's update for a client, from what happened."],
    ["Catch me up on an engagement", "Where an engagement stands: agreed, done, in progress, waiting."],
  ];
</script>

<SettingsLayout title="Using with AI">
  <Card.Root>
    <Card.Header><Card.Title>Talk to it like a colleague</Card.Title></Card.Header>
    <Card.Content class="flex flex-col gap-3 text-sm leading-relaxed">
      <p>Once your assistant is connected (<a href="/settings/help/mcp" class="underline underline-offset-4">Connecting an MCP</a>), you don’t need to know Runwell’s screens or ids. Say what you want, in your words, and it finds the {clients}, {engagements} and people by name. When a name could mean two things, it asks.</p>
      <p class="text-muted-foreground">It works as <strong>{agent}</strong> with your role, and shows you anything that would reach a {client} before doing it. It finishes {work} into <em>In review</em>; a person marks it done.</p>
    </Card.Content>
  </Card.Root>

  <Tabs.Root value={tabs.value} onValueChange={tabs.set} class="gap-6">
    <Tabs.List aria-label="What to ask">
      <Tabs.Trigger value="starts">Ready-made starts</Tabs.Trigger>
      <Tabs.Trigger value="projects">If you run projects</Tabs.Trigger>
      <Tabs.Trigger value="work">If you do the work</Tabs.Trigger>
      <Tabs.Trigger value="schedule">On a schedule</Tabs.Trigger>
      <Tabs.Trigger value="control">Staying in control</Tabs.Trigger>
    </Tabs.List>
    <Tabs.Content value="starts">
      <Card.Root>
        <Card.Header><Card.Title>Ready-made starts</Card.Title><Card.Description>Most assistants list these as prompts or shortcuts once Runwell is connected:</Card.Description></Card.Header>
        <Card.Content class="text-sm">
          <ul class="list-disc space-y-1 ps-5">{#each prompts as [title, description] (title)}<li><strong>{title}</strong> <span class="text-muted-foreground">{description}</span></li>{/each}</ul>
        </Card.Content>
      </Card.Root>
    </Tabs.Content>

    <Tabs.Content value="projects">
      <Card.Root>
        <Card.Header><Card.Title>If you run projects</Card.Title></Card.Header>
        <Card.Content class="text-sm">
          <ul class="list-disc space-y-1 ps-5">
            <li>“What needs me today? Most urgent first.”</li>
            <li>“Catch me up on the <em>Spanish pages</em> {engagement}: what’s agreed, done, in progress, and waiting on the {client}.”</li>
            <li>“Give the <em>holiday hours</em> {work} to Priya, due Friday.”</li>
            <li>“Triage the open requests: for each, suggest what it should become, and ask me before promoting any.”</li>
            <li>“Add <em>Spanish pages</em> as a change order on <em>WO-12</em> and send it to <em>Mia</em>.” <span class="text-muted-foreground">It shows you the change order first.</span></li>
            <li>“Draft this week’s update for <em>{client}</em> from what happened. Don’t send it.”</li>
            <li>“Before my call with <em>{client}</em>, summarise everything on their account since last week.”</li>
            <li>“Find overdue {work} and commitments, and draft a note to each owner.”</li>
          </ul>
        </Card.Content>
      </Card.Root>
    </Tabs.Content>

    <Tabs.Content value="work">
      <Card.Root>
        <Card.Header><Card.Title>If you do the work</Card.Title></Card.Header>
        <Card.Content class="text-sm">
          <ul class="list-disc space-y-1 ps-5">
            <li>“What am I on today?”</li>
            <li>“The hero images are done: put them in review and note what changed.”</li>
            <li>“Start on my next {work}.”</li>
            <li>“Ask Sarah whether Sunday hours are in scope.” <span class="text-muted-foreground">It leaves a note that mentions her.</span></li>
            <li>“Check my in-progress {work} against the code and move what’s actually finished to In review, with a note saying how you know.”</li>
          </ul>
        </Card.Content>
      </Card.Root>
    </Tabs.Content>

    <Tabs.Content value="schedule">
      <Card.Root>
        <Card.Header><Card.Title>On a schedule</Card.Title><Card.Description>Assistants that run tasks for you (a morning summary, a Friday check) can ask Runwell only what’s changed since they last looked, so they never repeat themselves.</Card.Description></Card.Header>
        <Card.Content class="text-sm">
          <ul class="list-disc space-y-1 ps-5">
            <li>“Every weekday at 8, message me what changed in Runwell overnight and what needs me today.”</li>
            <li>“Every Friday afternoon, list {engagements} with nothing done this week, and anything overdue.”</li>
            <li>“When a new request comes in from <em>{client}</em>, tell me, with your suggestion for what it should become.”</li>
          </ul>
        </Card.Content>
      </Card.Root>
    </Tabs.Content>

    <Tabs.Content value="control" class="flex flex-col gap-6">
      <Card.Root>
        <Card.Header><Card.Title>Your {clients}’ assistants</Card.Title></Card.Header>
        <Card.Content class="text-sm leading-relaxed">
          {cap(clients)} who connect their own assistant to their portal can ask it “Where are things at?”, “What’s waiting on me?” or “Ask them for a Spanish menu by Friday”, and it comes to you as a request, marked as sent through their assistant. Requests also arrive by email when the requests address is set in <a href="/settings/email" class="underline underline-offset-4">Settings › Email</a>.
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header><Card.Title>Staying in control</Card.Title></Card.Header>
        <Card.Content class="text-sm">
          <ul class="list-disc space-y-1 ps-5">
            <li><strong>See what it did</strong>: each connection in <a href="/settings/connected_apps" class="underline underline-offset-4">Connected apps</a> lists its recent calls, and the history on every record says what came through an assistant.</li>
            <li><strong>Pause or disconnect</strong> any connection there, at once. An owner can for anyone’s, {clients}’ included.</li>
            <li><strong>Read only</strong>: connect with read-only access when it should look and not change anything.</li>
            <li><strong>Nothing reaches a {client} unasked</strong>: sending, emailing and sharing always wait for your yes.</li>
          </ul>
        </Card.Content>
      </Card.Root>
    </Tabs.Content>
  </Tabs.Root>
</SettingsLayout>
