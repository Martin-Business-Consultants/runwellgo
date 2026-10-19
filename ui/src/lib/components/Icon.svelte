<script>
  // An icon by the admin's name for it, drawn from Lucide (shadcn's icons).
  import Activity from "@lucide/svelte/icons/activity";
  import Plus from "@lucide/svelte/icons/plus";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import ArrowRight from "@lucide/svelte/icons/arrow-right";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import BellRing from "@lucide/svelte/icons/bell-ring";
  import Zap from "@lucide/svelte/icons/zap";
  import Briefcase from "@lucide/svelte/icons/briefcase";
  import Calendar from "@lucide/svelte/icons/calendar";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import Check from "@lucide/svelte/icons/check";
  import ChefHat from "@lucide/svelte/icons/chef-hat";
  import Clipboard from "@lucide/svelte/icons/clipboard";
  import CircleX from "@lucide/svelte/icons/circle-x";
  import X from "@lucide/svelte/icons/x";
  import Code from "@lucide/svelte/icons/code";
  import PanelLeftClose from "@lucide/svelte/icons/panel-left-close";
  import Copy from "@lucide/svelte/icons/copy";
  import FileText from "@lucide/svelte/icons/file-text";
  import Download from "@lucide/svelte/icons/download";
  import Mail from "@lucide/svelte/icons/mail";
  import Users from "@lucide/svelte/icons/users";
  import ExternalLink from "@lucide/svelte/icons/external-link";
  import GripVertical from "@lucide/svelte/icons/grip-vertical";
  import LayoutGrid from "@lucide/svelte/icons/layout-grid";
  import Info from "@lucide/svelte/icons/info";
  import History from "@lucide/svelte/icons/history";
  import House from "@lucide/svelte/icons/house";
  import Image from "@lucide/svelte/icons/image";
  import ImagePlus from "@lucide/svelte/icons/image-plus";
  import Lightbulb from "@lucide/svelte/icons/lightbulb";
  import Link from "@lucide/svelte/icons/link";
  import Lock from "@lucide/svelte/icons/lock";
  import LogOut from "@lucide/svelte/icons/log-out";
  import MapPin from "@lucide/svelte/icons/map-pin";
  import Megaphone from "@lucide/svelte/icons/megaphone";
  import Menu from "@lucide/svelte/icons/menu";
  import Monitor from "@lucide/svelte/icons/monitor";
  import Moon from "@lucide/svelte/icons/moon";
  import Move from "@lucide/svelte/icons/move";
  import Newspaper from "@lucide/svelte/icons/newspaper";
  import Package from "@lucide/svelte/icons/package";
  import KeyRound from "@lucide/svelte/icons/key-round";
  import User from "@lucide/svelte/icons/user";
  import Pin from "@lucide/svelte/icons/pin";
  import Quote from "@lucide/svelte/icons/quote";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import Search from "@lucide/svelte/icons/search";
  import Settings from "@lucide/svelte/icons/settings";
  import SlidersHorizontal from "@lucide/svelte/icons/sliders-horizontal";
  import Layers from "@lucide/svelte/icons/layers";
  import Sun from "@lucide/svelte/icons/sun";
  import ToggleLeft from "@lucide/svelte/icons/toggle-left";
  import Tag from "@lucide/svelte/icons/tag";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import Upload from "@lucide/svelte/icons/upload";
  import Globe from "@lucide/svelte/icons/globe";
  import Wrench from "@lucide/svelte/icons/wrench";
  import Type from "@lucide/svelte/icons/type";
  import Phone from "@lucide/svelte/icons/phone";
  import Pilcrow from "@lucide/svelte/icons/pilcrow";
  import Hash from "@lucide/svelte/icons/hash";
  import SquareChevronDown from "@lucide/svelte/icons/square-chevron-down";
  import CircleDot from "@lucide/svelte/icons/circle-dot";
  import ListChecks from "@lucide/svelte/icons/list-checks";
  import SquareCheck from "@lucide/svelte/icons/square-check";
  import EyeOff from "@lucide/svelte/icons/eye-off";
  import Heading from "@lucide/svelte/icons/heading";
  import ClipboardList from "@lucide/svelte/icons/clipboard-list";
  import Inbox from "@lucide/svelte/icons/inbox";
  import Send from "@lucide/svelte/icons/send";

  const icons = {
    activity: Activity, add: Plus, ai: Sparkles, "arrow-left": ArrowLeft, "arrow-right": ArrowRight, "arrow-up": ArrowUp,
    "bell-alert": BellRing, bolt: Zap, briefcase: Briefcase, calendar: Calendar, "caret-down": ChevronDown,
    "check-circle": CircleCheck, check: Check, "chef-hat": ChefHat, clipboard: Clipboard, "close-circle": CircleX,
    close: X, code: Code, "collapse-menu": PanelLeftClose, "copy-paste": Copy, document: FileText, download: Download,
    email: Mail, everyone: Users, external: ExternalLink, grip: GripVertical, grid: LayoutGrid, "help-circle": Info,
    history: History, home: House, image: Image, lightbulb: Lightbulb, link: Link, lock: Lock, logout: LogOut,
    marker: MapPin, megaphone: Megaphone, menu: Menu, monitor: Monitor, moon: Moon, move: Move, newspaper: Newspaper,
    package: Package, password: KeyRound, person: User, "picture-add": ImagePlus, pinned: Pin, quote: Quote,
    refresh: RefreshCw, search: Search, settings: Settings, "settings-outline": Settings, sliders: SlidersHorizontal,
    "text": Type, "phone": Phone, "paragraph": Pilcrow, "hash": Hash, "dropdown": SquareChevronDown, "radio": CircleDot, "checkboxes": ListChecks, "check-square": SquareCheck, "eye-off": EyeOff, "heading": Heading, "form": ClipboardList, "inbox": Inbox, "send": Send,
    stack: Layers, sun: Sun, switch: ToggleLeft, tag: Tag, trash: Trash2, upload: Upload, world: Globe, wrench: Wrench,
  };

  /** @type {{ name: string, size?: "xs"|"sm"|"md"|"lg"|"xl", class?: string }} */
  let { name, size = "sm", class: klass = "" } = $props();
  const sizes = { xs: "size-3", sm: "size-4", md: "size-5", lg: "size-6", xl: "size-10" };
  const Glyph = $derived(icons[name]);
</script>

{#if Glyph}<Glyph class="shrink-0 {sizes[size]} {klass}" aria-hidden="true" />{/if}
