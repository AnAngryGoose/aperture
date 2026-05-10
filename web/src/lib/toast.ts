import { writable } from 'svelte/store';

export type ToastKind = 'info' | 'success' | 'error';
export interface Toast { id: number; message: string; kind: ToastKind; }

let next = 0;
const { subscribe, update } = writable<Toast[]>([]);

function add(message: string, kind: ToastKind, durationMs = 4000) {
	const id = ++next;
	update(ts => [...ts, { id, message, kind }]);
	setTimeout(() => remove(id), durationMs);
}

function remove(id: number) {
	update(ts => ts.filter(t => t.id !== id));
}

export const toast = {
	subscribe,
	info:    (msg: string) => add(msg, 'info'),
	success: (msg: string) => add(msg, 'success'),
	error:   (msg: string, dur = 6000) => add(msg, 'error', dur),
	remove
};
