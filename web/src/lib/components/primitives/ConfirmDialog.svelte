<script lang="ts">
	import Modal from './Modal.svelte';
	import Button from './Button.svelte';
	import Icon from './Icon.svelte';

	type Tone = 'warning' | 'danger';

	interface Props {
		open: boolean;
		title: string;
		/** Short, declarative summary line. */
		message: string;
		/** Optional secondary line — usually names the affected resource. */
		detail?: string;
		/** Optional checklist of "what will happen" bullets. */
		consequences?: string[];
		tone?: Tone;
		confirmLabel?: string;
		cancelLabel?: string;
		busy?: boolean;
		onconfirm: () => void;
		oncancel: () => void;
	}

	let {
		open,
		title,
		message,
		detail,
		consequences,
		tone = 'danger',
		confirmLabel = 'Confirm',
		cancelLabel = 'Cancel',
		busy = false,
		onconfirm,
		oncancel
	}: Props = $props();
</script>

<Modal {open} onclose={() => { if (!busy) oncancel(); }} title={undefined} width="440px">
	<div class="confirm tone-{tone}">
		<div class="head">
			<span class="icon" aria-hidden="true">
				<Icon name="warn" size={18} />
			</span>
			<h2 class="title">{title}</h2>
		</div>
		<p class="message">{message}</p>
		{#if detail}
			<p class="detail mono">{detail}</p>
		{/if}
		{#if consequences && consequences.length > 0}
			<ul class="consequences">
				{#each consequences as c}
					<li>{c}</li>
				{/each}
			</ul>
		{/if}
		<div class="actions">
			<Button variant="ghost" onclick={oncancel} disabled={busy}>{cancelLabel}</Button>
			<Button variant={tone} onclick={onconfirm} loading={busy}>
				{busy ? 'Working…' : confirmLabel}
			</Button>
		</div>
	</div>
</Modal>

<style>
	.confirm { display: flex; flex-direction: column; gap: 10px; }

	.head {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.icon {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 30px;
		height: 30px;
		border-radius: 50%;
	}
	.tone-warning .icon { background: var(--warn-soft); color: var(--warn); }
	.tone-danger .icon { background: var(--crit-soft); color: var(--crit); }

	.title {
		margin: 0;
		font-size: 15px;
		font-weight: 600;
		color: var(--text);
		letter-spacing: -0.01em;
	}

	.message {
		margin: 0;
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-dim);
	}

	.detail {
		margin: 0;
		font-size: 12px;
		color: var(--text);
		background: var(--bg-elev-2);
		border: 1px solid var(--line);
		border-radius: var(--r-md);
		padding: 8px 10px;
		word-break: break-all;
	}

	.consequences {
		margin: 4px 0 0;
		padding-left: 18px;
		font-size: 12px;
		color: var(--text-dim);
		line-height: 1.55;
	}
	.consequences li { margin: 2px 0; }

	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
		margin-top: 10px;
	}
</style>
