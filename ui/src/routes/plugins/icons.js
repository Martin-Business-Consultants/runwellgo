// The icons a plugin can name for its nav page and quick actions (lucide
// names); anything else is a puzzle piece. A plugin ships no code, so the
// set is the app's.
import Puzzle from "@lucide/svelte/icons/puzzle";
import Clock from "@lucide/svelte/icons/clock";
import History from "@lucide/svelte/icons/history";
import Timer from "@lucide/svelte/icons/timer";
import Hand from "@lucide/svelte/icons/hand";
import Calendar from "@lucide/svelte/icons/calendar";
import ChartBar from "@lucide/svelte/icons/chart-bar";
import ChartLine from "@lucide/svelte/icons/chart-line";
import FileText from "@lucide/svelte/icons/file-text";
import Users from "@lucide/svelte/icons/users";
import Briefcase from "@lucide/svelte/icons/briefcase";
import ListChecks from "@lucide/svelte/icons/list-checks";
import Star from "@lucide/svelte/icons/star";
import Bell from "@lucide/svelte/icons/bell";
import Settings from "@lucide/svelte/icons/settings";
import Globe from "@lucide/svelte/icons/globe";
import DollarSign from "@lucide/svelte/icons/dollar-sign";
import Receipt from "@lucide/svelte/icons/receipt";
import Mail from "@lucide/svelte/icons/mail";
import Inbox from "@lucide/svelte/icons/inbox";
import BookOpen from "@lucide/svelte/icons/book-open";
import Code from "@lucide/svelte/icons/code";
import Zap from "@lucide/svelte/icons/zap";
import Shield from "@lucide/svelte/icons/shield";
import Sun from "@lucide/svelte/icons/sun";
import Gauge from "@lucide/svelte/icons/gauge";
import Megaphone from "@lucide/svelte/icons/megaphone";
import Plug from "@lucide/svelte/icons/plug";
import Plus from "@lucide/svelte/icons/plus";
import MessageSquare from "@lucide/svelte/icons/message-square";
import MessagesSquare from "@lucide/svelte/icons/messages-square";
import CheckCheck from "@lucide/svelte/icons/check-check";

const icons = {
  puzzle: Puzzle, clock: Clock, history: History, timer: Timer, hand: Hand, calendar: Calendar, "chart-bar": ChartBar,
  "chart-line": ChartLine, "file-text": FileText, users: Users, briefcase: Briefcase, "list-checks": ListChecks, star: Star,
  bell: Bell, settings: Settings, globe: Globe, "dollar-sign": DollarSign, receipt: Receipt, mail: Mail, inbox: Inbox,
  "book-open": BookOpen, code: Code, zap: Zap, shield: Shield, sun: Sun, gauge: Gauge, megaphone: Megaphone, plug: Plug, plus: Plus,
  "message-square": MessageSquare, "messages-square": MessagesSquare, "check-check": CheckCheck,
};

export const pluginIcon = (name) => icons[name] || Puzzle;
