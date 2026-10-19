<script>
  // One Ask conversation, for staff and for a client in their portal: a
  // greeting and four questions worth a click while it's empty; the
  // messages; the answer as it's written (streamed to the tab, read again
  // now and then where the stream can't stay open); a button back down
  // when you've scrolled up; and the box to ask in.
  import { onDestroy } from "svelte";
  import { Button } from "$lib/components/ui/button/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import ArrowDown from "@lucide/svelte/icons/arrow-down";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import AskMessage from "./AskMessage.svelte";
  import AskMarkdown from "./AskMarkdown.svelte";
  import AskComposer from "./AskComposer.svelte";
  import { staff, portal, status } from "./ai.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { subscribe, live } from "$lib/live.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";

  let { record = "", page = "", mode = "staff", chatId = null, onchat = null, autofocus = true } = $props();
  // svelte-ignore state_referenced_locally
  const inPortal = mode === "portal";
  const api = inPortal ? portal : staff;

  let data = $state(null), busy = $state(false), error = $state("");
  let draft = $state(null); // the answer being written: {text, thinking, stale}
  let scroller = $state(null), composer = $state(null), atBottom = $state(true);
  let timer = null;

  export function focus() { composer?.focus(); }

  function show(d) {
    data = d;
    if (!d.replying) draft = null;
    onchat?.(d.chat);
    poll();
    stick();
    if (autofocus && !d.replying && !d.waiting) queueMicrotask(() => composer?.focus());
  }
  // Read again while it answers: rarely while the stream is open (it says
  // when to), every second where it can't be.
  function poll() {
    clearTimeout(timer);
    if (data?.replying) timer = setTimeout(refresh, live.connected && !inPortal ? 5000 : 1000);
  }
  async function run(fn) {
    busy = true;
    error = "";
    try { show(await fn()); } catch (e) { error = e.message; }
    busy = false;
  }
  const q = () => `?record=${encodeURIComponent(record || "")}&page=${encodeURIComponent(page || "")}`;
  const refresh = () => data && api.get(`/chats/${data.chat.id}`).then(show).catch((e) => (error = e.message));
  // svelte-ignore state_referenced_locally
  run(() => (chatId ? api.get(`/chats/${chatId}`) : api.get(`/chats/current${q()}`)));
  export function fresh() { draft = null; return run(() => api.post("/chats", { record, page })); }

  // The words as they're written, pushed to this tab.
  const stop = inPortal ? null : subscribe("ai", (topic, d) => {
    if (topic === "resync") { if (data?.replying) refresh(); return; }
    if (topic !== "ask" || !data || d.chat_id !== data.chat.id) return;
    if (d.state === "thinking") { draft = { text: "", thinking: "" }; data.replying = true; }
    else if (d.state === "writing") {
      if (d.stale) { refresh(); return; }
      draft = { text: d.text || "", thinking: d.thinking || "" };
      stick();
    } else if (d.state === "saved" || d.state === "done") refresh();
  });
  onDestroy(() => { clearTimeout(timer); stop?.(); });

  function stick() {
    if (!atBottom) return;
    queueMicrotask(() => requestAnimationFrame(() => { if (scroller) scroller.scrollTop = scroller.scrollHeight; }));
  }
  function onscroll() {
    atBottom = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 48;
  }
  function toBottom() {
    const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    scroller.scrollTo({ top: scroller.scrollHeight, behavior: reduce ? "auto" : "smooth" });
  }

  async function send({ text, images, agent }) {
    if (!data) return false;
    atBottom = true;
    if (agent && !inPortal) {
      busy = true;
      error = "";
      try {
        const r = await staff.post(`/agents/${agent.id}/runs`, { prompt: text, record: data.chat.record || record || "" });
        show(await staff.post(`/chats/${data.chat.id}/agent_card`, { card: { run_id: r.run.id, prompt: text } }));
        toast(`@${agent.handle} is on it.`);
        busy = false;
        return true;
      } catch (e) { error = e.message; busy = false; return false; }
    }
    busy = true;
    error = "";
    try {
      const d = inPortal ? await api.post(`/chats/${data.chat.id}/messages`, { message: { content: text } })
        : await api.post(`/chats/${data.chat.id}/send`, { message: { content: text, images } });
      if (d.replying) draft = { text: "", thinking: "" };
      show(d);
      if (!inPortal) status.load(true);
      busy = false;
      return true;
    } catch (e) { error = e.message; busy = false; return false; }
  }
  const stopAnswer = () => run(() => staff.post(`/chats/${data.chat.id}/stop`));
  const regenerate = () => { atBottom = true; draft = { text: "", thinking: "" }; return run(() => staff.post(`/chats/${data.chat.id}/regenerate`)); };
  const edit = (content) => { atBottom = true; draft = { text: "", thinking: "" }; return run(() => staff.post(`/chats/${data.chat.id}/edit`, { message: { content } })); };
  const decide = (call, decision) => run(() => api.post(`/chats/${data.chat.id}/decisions`, { tool_call_id: call.tool_call_id, decision }));
  const pickModel = (model) => run(() => staff.patch(`/chats/${data.chat.id}`, { chat: { model } }));
  async function rate(m, rating) {
    try {
      const r = await staff.post(`/chats/${data.chat.id}/messages/${m.id}/feedback`, { rating });
      m.feedback = r.rating;
      toast(r.message);
    } catch (e) { toastError(e); }
  }

  const empty = $derived(data && data.messages.length === 0);
  const firstName = $derived((session.me?.name || "").split(" ")[0]);
  function greeting() {
    const h = new Date().getHours();
    return h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
  }
</script>

<div class="relative flex min-h-0 flex-1 flex-col">
  <div bind:this={scroller} {onscroll} class="min-h-0 flex-1 overflow-y-auto overscroll-contain" aria-live="polite" aria-busy={data?.replying ? "true" : "false"}>
    <div class="mx-auto flex w-full max-w-3xl flex-col gap-5 px-4 py-5">
      {#if !data}
        <Skeleton class="h-16 w-full" /><Skeleton class="h-24 w-3/4" />
      {:else if empty && !data.replying}
        <div class="flex flex-col gap-1 pt-4 sm:pt-10">
          <p class="text-xl font-semibold tracking-tight sm:text-2xl">{inPortal ? "Hello" : `${greeting()}${firstName ? ", " + firstName : ""}`}.</p>
          <p class="text-muted-foreground text-base sm:text-lg">
            {#if inPortal}What would you like to know about your work with us?
            {:else if data.chat.record}What would you like to know about {data.chat.heading}?
            {:else}What can I help with?{/if}
          </p>
        </div>
        {#if !data.unavailable && data.prompts.length}
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
            {#each data.prompts as p (p)}
              <button type="button" class="hover:bg-accent focus-visible:ring-ring/50 flex flex-col rounded-xl border px-3.5 py-3 text-left text-sm outline-none focus-visible:ring-[3px] disabled:opacity-50"
                onclick={() => send({ text: p, images: [] })} disabled={busy}>
                <span class="font-medium">{p}</span>
              </button>
            {/each}
          </div>
        {/if}
        <p class="text-muted-foreground text-xs">
          {#if inPortal}It answers from your portal. It can send a request for the team too, once you approve it.
          {:else}It answers from Runwell as you, and asks before changing anything. Type @ to hand a question to one of your agents.{/if}
        </p>
      {:else}
        {#each data.messages as m, i (m.id)}
          <AskMessage {m} chatId={data.chat.id} first={i === 0 || data.messages[i - 1].role === "user"} portal={inPortal} {busy}
            lastQuestion={!data.replying && !data.waiting && m.id === data.last_question_id}
            lastAnswer={!data.replying && !data.waiting && m.id === data.last_answer_id && i === data.messages.length - 1}
            ondecide={decide} onrate={rate} onregenerate={regenerate} onedit={edit} />
        {/each}
        {#if data.replying}
          <div class="flex gap-2.5">
            {#if data.messages.at(-1)?.role === "assistant"}<span class="w-7 shrink-0" aria-hidden="true"></span>
            {:else}<span class="bg-background text-primary mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full border" aria-hidden="true"><Sparkles class="size-3.5" /></span>{/if}
            <div class="flex min-w-0 flex-1 flex-col gap-2">
              {#if draft?.thinking}
                <p class="text-muted-foreground line-clamp-3 border-s-2 ps-3 text-xs whitespace-pre-wrap">{draft.thinking}</p>
              {/if}
              {#if draft?.text}
                <AskMarkdown text={draft.text} portal={inPortal} />
              {:else}
                <p class="text-muted-foreground animate-pulse text-sm font-medium motion-reduce:animate-none">Thinking…</p>
              {/if}
            </div>
          </div>
        {/if}
      {/if}
    </div>
  </div>

  {#if data}
    <div class="relative mx-auto flex w-full max-w-3xl flex-col gap-2 px-4 pt-2 pb-3">
      {#if !atBottom && !empty}
        <Button variant="outline" size="icon-sm" class="bg-background absolute -top-10 left-1/2 z-10 -translate-x-1/2 rounded-full shadow-sm" onclick={toBottom} aria-label="Scroll to the latest" title="Scroll to the latest"><ArrowDown /></Button>
      {/if}
      {#if data.unavailable}
        <Alert.Root>
          <Alert.Description>
            {data.unavailable}
            {#if !inPortal && session.can("manage_settings") && !data.unavailable.includes("kept out")} <a href="/settings/ai" class="text-primary underline underline-offset-4">Settings › AI</a>.{/if}
          </Alert.Description>
        </Alert.Root>
      {/if}
      {#if error}<p class="text-destructive text-sm" role="alert">{error}</p>{/if}
      {#if data.waiting}
        <p class="text-muted-foreground rounded-md border px-3 py-2 text-sm">Approve or decline the change above, and it carries on.</p>
      {:else if !data.unavailable}
        <AskComposer bind:this={composer} portal={inPortal} {busy} replying={data.replying} images={data.images} providerName={data.provider_name}
          model={data.chat.model} modelName={data.chat.model_name} onmodel={inPortal ? null : pickModel}
          onsend={send} onstop={inPortal ? null : stopAnswer}
          placeholder={inPortal ? "Ask a question…" : "Ask, or ask it to do something… (@ for an agent)"} />
      {/if}
    </div>
  {/if}
</div>
