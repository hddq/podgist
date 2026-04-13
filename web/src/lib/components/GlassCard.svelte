<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		class: className = '',
		children
	}: {
		class?: string;
		children?: Snippet;
	} = $props();
</script>

<div
	class={`rounded-2xl border border-zinc-800 bg-zinc-900/40 shadow-2xl backdrop-blur-md transition-colors hover:border-zinc-700/80 ${className}`}
>
	{@render children?.()}
</div>