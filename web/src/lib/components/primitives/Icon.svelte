<script lang="ts">
	import {
		LayoutDashboard, Server, Container, Layers, HardDrive, Network,
		ScrollText, Terminal, Cpu, Bell, Settings, Search, RefreshCw,
		Sun, Moon, X, Plus, MoreHorizontal, ChevronRight, ChevronDown,
		AlertTriangle, AlertCircle, Info, CheckCircle, Wifi, WifiOff,
		Thermometer, Activity, MemoryStick, Database, Box, Play, Square,
		RotateCcw, Pause, Trash2, ExternalLink, Copy, Eye, EyeOff,
		LogOut, Key, Shield, Clock, Zap, GitBranch, Package,
		ArrowDown, ArrowUp, ArrowLeft, Gauge, List, LayoutGrid, Rows3,
		Siren, Radio, Bot
	} from 'lucide-svelte';

	const ICONS: Record<string, typeof Server> = {
		dashboard: LayoutDashboard,
		hosts: Server,
		containers: Container,
		stacks: Layers,
		storage: HardDrive,
		network: Network,
		logs: ScrollText,
		shell: Terminal,
		automation: Bot,
		alerts: Siren,
		settings: Settings,
		search: Search,
		refresh: RefreshCw,
		sun: Sun,
		moon: Moon,
		close: X,
		x: X,
		plus: Plus,
		more: MoreHorizontal,
		'chevron-right': ChevronRight,
		'chevron-down': ChevronDown,
		warn: AlertTriangle,
		crit: AlertCircle,
		info: Info,
		ok: CheckCircle,
		online: Wifi,
		offline: WifiOff,
		temp: Thermometer,
		cpu: Activity,
		mem: MemoryStick,
		disk: Database,
		container: Box,
		play: Play,
		stop: Square,
		restart: RotateCcw,
		pause: Pause,
		trash: Trash2,
		external: ExternalLink,
		copy: Copy,
		eye: Eye,
		'eye-off': EyeOff,
		logout: LogOut,
		key: Key,
		shield: Shield,
		clock: Clock,
		zap: Zap,
		branch: GitBranch,
		package: Package,
		'arrow-down': ArrowDown,
		'arrow-up': ArrowUp,
		'arrow-left': ArrowLeft,
		gauge: Gauge,
		list: List,
		grid: LayoutGrid,
		rows: Rows3,
		docker: Container,
		linux: Server,
		edge: Radio,
	};

	interface Props {
		name: string;
		size?: number;
		class?: string;
	}

	let { name, size = 16, class: cls = '' }: Props = $props();
	const Component = $derived(ICONS[name] ?? Info);
</script>

<Component size={size} class={cls} stroke-width={1.5} />
