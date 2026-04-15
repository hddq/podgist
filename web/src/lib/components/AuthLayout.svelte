<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		title,
		subtitle,
		error = '',
		children,
		footer
	}: {
		title: string;
		subtitle: string;
		error?: string;
		children: Snippet;
		footer?: Snippet;
	} = $props();
</script>

<div class="relative flex min-h-screen items-center justify-center overflow-hidden bg-black px-4 py-10 text-zinc-100 sm:px-6 lg:px-8">
	<div
		class="pointer-events-none absolute left-[10%] top-[-10%] z-0 h-[420px] w-[420px] rounded-full bg-emerald-600/10 blur-[120px]"
	></div>
	<div
		class="pointer-events-none absolute bottom-[-12%] right-[8%] z-0 h-[520px] w-[520px] rounded-full bg-indigo-600/10 blur-[150px]"
	></div>

	<div class="relative z-10 w-full max-w-md">
		<div class="rounded-2xl border border-zinc-800 bg-zinc-900/40 shadow-2xl backdrop-blur-md">
			<div class="flex flex-col gap-4 p-6 sm:p-8">
				<div class="mb-1 text-center">
					<div
						class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-2xl border border-emerald-500/30 bg-emerald-500/10 text-xl"
					>
						🎙
					</div>
					<h1 class="text-3xl font-bold tracking-tight text-zinc-100">Podgist</h1>
					<p class="mt-1 text-sm text-zinc-400">{subtitle}</p>
				</div>

				{#if error}
					<div role="alert" class="rounded-xl border border-red-500/40 bg-red-500/10 px-4 py-3 text-sm text-red-200">
						<span>{error}</span>
					</div>
				{/if}

				<h2 class="sr-only">{title}</h2>

				<div class="flex flex-col gap-4">
					{@render children()}
				</div>

				{#if footer}
					<div class="mt-2 border-t border-zinc-800 pt-4">
						{@render footer()}
					</div>
				{/if}
			</div>
		</div>
	</div>
</div>
