<script>
  // Settings › Appearance: the theme everyone starts on, colors, corners,
  // font and text size (each saved, and shown, as it's picked), custom CSS
  // with a brief to hand an AI, and the logo and favicon: three tabs
  // (?tab=), Look, Custom CSS, and Logo and favicon.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as RadioGroup from "$lib/components/ui/radio-group/index.js";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Textarea } from "$lib/components/ui/textarea/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Monitor from "@lucide/svelte/icons/monitor";
  import Sun from "@lucide/svelte/icons/sun";
  import Moon from "@lucide/svelte/icons/moon";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import ImagePlus from "@lucide/svelte/icons/image-plus";
  import { api, get, patch } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { pageTabs } from "$lib/page-tabs.svelte.js";
  import { appearance, SCHEME_SWATCHES, FONT_STACKS, RADIUS_VALUES } from "$lib/appearance.svelte.js";

  let data = $state(null), css = $state(""), everywhere = $state(false), cssError = $state(""), busy = $state(false), copied = $state(false);
  let version = $state(Date.now());
  const tabs = pageTabs(["look", "css", "images"], { hashes: { "custom-css": "css" } });

  function fill(d) {
    data = d;
    css = d.custom_css || "";
    everywhere = d.custom_css_everywhere;
  }
  get("/settings/appearance").then(fill).then(tabs.scrollToHash).catch(toastError);

  async function save(setting, extra = {}) {
    try {
      const res = await patch("/settings/appearance", { setting, ...extra });
      fill(res); toast(res.summary);
      await appearance.load();
      if ("theme" in setting) appearance.resetTheme();
      return true;
    } catch (err) {
      if (err.errors?.custom_css) cssError = `Custom CSS ${err.errors.custom_css.join(", ")}. Nothing was saved.`;
      else toastError(err);
      return false;
    }
  }
  async function saveCSS(e) {
    e.preventDefault();
    busy = true; cssError = "";
    await save({ custom_css: css, custom_css_everywhere: everywhere });
    busy = false;
  }
  async function upload(name, e) {
    const file = e.currentTarget.files?.[0];
    if (!file) return;
    const form = new FormData();
    form.append(name, file);
    try { const res = await api("PATCH", "/settings/appearance", form); fill(res); toast(res.summary); version = Date.now(); await appearance.load(); }
    catch (err) { toastError(err); }
    e.currentTarget.value = "";
  }
  async function remove(name) {
    if (await save({}, { remove: name })) version = Date.now();
  }
  async function copyBrief() {
    try {
      const res = await fetch("/api/settings/theme_brief?format=text", { credentials: "same-origin" });
      if (!res.ok) throw new Error("Couldn’t get the brief.");
      await navigator.clipboard.writeText(await res.text());
      copied = true; setTimeout(() => (copied = false), 1600);
      toast("The brief is copied. Paste it into any AI.");
    } catch (err) { toastError(err); }
  }

  const themeIcons = { system: Monitor, light: Sun, dark: Moon };
  const sizes = { small: "0.85em", default: "1em", large: "1.15em", larger: "1.3em" };
  const corners = { sharp: "0.15em", fizzy: "0.5em", round: "0.9em" };
</script>

{#snippet choices(key, title, description)}
  <Card.Root>
    <Card.Header>
      <Card.Title>{title}</Card.Title>
      {#if description}<Card.Description>{description}</Card.Description>{/if}
    </Card.Header>
    <Card.Content>
      <RadioGroup.Root value={data[key]} onValueChange={(v) => save({ [key]: v })} class="grid grid-cols-2 gap-3 sm:grid-cols-5">
        {#each data.choices[key] as value (value)}
          <Field.Label for={`${key}-${value}`} class="w-full has-data-[state=checked]:border-primary has-data-[state=checked]:bg-muted flex cursor-pointer flex-col items-center gap-2 rounded-md border p-3">
            <RadioGroup.Item {value} id={`${key}-${value}`} class="sr-only" />
            {#if key === "theme"}
              {@const Icon = themeIcons[value]}<Icon class="size-5" />
            {:else if key === "scheme"}
              <span class="size-6 rounded-full" style:background={SCHEME_SWATCHES[value]} aria-hidden="true"></span>
            {:else if key === "radius"}
              <span class="border-foreground size-6 border-2" style:border-radius={corners[value]} title={RADIUS_VALUES[value]} aria-hidden="true"></span>
            {:else if key === "font"}
              <span class="text-lg font-semibold" style:font-family={FONT_STACKS[value]} aria-hidden="true">Aa</span>
            {:else}
              <span class="font-semibold" style:font-size={sizes[value]} aria-hidden="true">Aa</span>
            {/if}
            <span class="text-xs">{data.names[key][value]}</span>
          </Field.Label>
        {/each}
      </RadioGroup.Root>
    </Card.Content>
  </Card.Root>
{/snippet}

<SettingsLayout title="Appearance">
  {#if !data}
    <Skeleton class="h-96 w-full" />
  {:else}
    <Tabs.Root value={tabs.value} onValueChange={tabs.set} class="gap-6">
      <Tabs.List aria-label="Appearance">
        <Tabs.Trigger value="look">Look</Tabs.Trigger>
        <Tabs.Trigger value="css">Custom CSS</Tabs.Trigger>
        <Tabs.Trigger value="images">Logo and favicon</Tabs.Trigger>
      </Tabs.List>
      <Tabs.Content value="look" class="flex flex-col gap-6">
        {@render choices("theme", "Theme", "Where everyone starts. Anyone can still flip it with the sun and moon in their menu; picking one here also resets yours.")}
        {@render choices("scheme", "Colors")}
        {@render choices("radius", "Corners")}
        {@render choices("font", "Font")}
        {@render choices("text_size", "Text size", "Where everyone starts. Each person can choose their own in Settings › Your account.")}
      </Tabs.Content>

      <Tabs.Content value="css">
        <Card.Root id="custom-css">
          <Card.Header>
            <Card.Title>Custom CSS</Card.Title>
            <Card.Description>Make Runwell your own by giving its design tokens new values: colors, fonts, corners. Copy the brief, paste it into any AI with your logo or website, and paste its answer here. Add <code>?theme=off</code> to any address to see a page without it.</Card.Description>
            <Card.Action><Button variant="outline" size="sm" onclick={copyBrief}><Sparkles />{copied ? "Copied" : "Copy brief for AI"}</Button></Card.Action>
          </Card.Header>
          <Card.Content>
            <form onsubmit={saveCSS}>
              <Field.Group>
                {#if cssError}<p class="text-destructive text-sm">{cssError}</p>{/if}
                <Textarea bind:value={css} rows={12} spellcheck="false" autocomplete="off" class="font-mono text-xs" placeholder={":root {\n  --primary: oklch(0.58 0.14 40);\n  --radius: 0.75rem;\n}"} aria-label="Custom CSS" />
                <Field.Field orientation="horizontal">
                  <Checkbox id="css-everywhere" bind:checked={everywhere} />
                  <Field.Label for="css-everywhere" class="font-normal">Use it on {session.term("client").toLowerCase()} pages too: the portal and approvals</Field.Label>
                </Field.Field>
                <div><Button type="submit" disabled={busy}>Save CSS</Button></div>
              </Field.Group>
            </form>
          </Card.Content>
        </Card.Root>
      </Tabs.Content>

      <Tabs.Content value="images">
        <div class="grid gap-4 md:grid-cols-2">
          {#each [["logo", "Logo", "Shown in the menu in place of the Runwell mark. A wide PNG or SVG works best."], ["favicon", "Favicon", "The icon in the browser tab. A square PNG, at least 64 pixels."]] as [name, title, hint] (name)}
            <Card.Root>
              <Card.Header><Card.Title>{title}</Card.Title><Card.Description>{hint}</Card.Description></Card.Header>
              <Card.Content class="flex flex-col gap-3">
                {#if data[name]}
                  <div class="flex items-center gap-3">
                    <img src={`/api/appearance/${name}?v=${version}`} alt={title} class={name === "logo" ? "max-h-12 max-w-48" : "size-10"} />
                    <Button variant="ghost" size="sm" class="text-destructive" onclick={() => remove(name)}>Remove</Button>
                  </div>
                {/if}
                <Button variant="outline" onclick={() => document.getElementById(`${name}-file`)?.click()}><ImagePlus />{data[name] ? "Replace" : "Choose an image…"}</Button>
                <input id={`${name}-file`} type="file" accept="image/png, image/svg+xml, image/jpeg, image/webp, image/x-icon" class="hidden" onchange={(e) => upload(name, e)} />
              </Card.Content>
            </Card.Root>
          {/each}
        </div>
      </Tabs.Content>
    </Tabs.Root>
  {/if}
</SettingsLayout>
