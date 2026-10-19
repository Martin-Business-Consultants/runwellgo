<script>
  // Chat in the header slot: nothing to see, but it starts chat's state on
  // every page so the sidebar's unread count, presence and alerts work
  // wherever someone is in the app.
  import { chat } from "./chat.svelte.js";
  chat.start();
</script>
