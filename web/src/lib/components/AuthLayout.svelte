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

<div class="flex min-h-screen items-center justify-center bg-base-100 px-4">
	<div class="card w-full max-w-sm bg-base-200 shadow-xl">
		<div class="card-body gap-4">
			<div class="mb-2 text-center">
				<h1 class="text-3xl font-bold text-primary">🎙 Podgist</h1>
				<p class="mt-1 text-sm text-base-content/60">{subtitle}</p>
			</div>

			{#if error}
				<div role="alert" class="alert alert-error py-2 text-sm">
					<span>{error}</span>
				</div>
			{/if}

			<h2 class="sr-only">{title}</h2>

			{@render children()}
			{#if footer}
				{@render footer()}
			{/if}
		</div>
	</div>
</div>
