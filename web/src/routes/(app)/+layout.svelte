<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { auth } from '$lib/auth.svelte';
	import Sidebar from '$lib/components/Sidebar.svelte';

	let { children } = $props();

	let checking = $state(true);

	$effect(() => {
		if (!auth.checked) {
			auth.check().then((user) => {
				checking = false;
				if (!user) goto(`${base}/login`, { replaceState: true });
			});
		} else if (!auth.user) {
			goto(`${base}/login`, { replaceState: true });
		} else {
			checking = false;
		}
	});

</script>

{#if checking}
	<div class="flex h-screen items-center justify-center">
		<span class="loading loading-spinner loading-lg text-primary"></span>
	</div>
{:else}
	<div class="relative flex min-h-screen overflow-hidden bg-black text-zinc-100 font-sans">
		<div
			class="pointer-events-none absolute left-[10%] top-[-10%] z-0 h-[500px] w-[500px] rounded-full bg-emerald-600/10 blur-[120px]"
		></div>
		<div
			class="pointer-events-none absolute bottom-[-10%] right-[10%] z-0 h-[600px] w-[600px] rounded-full bg-indigo-600/10 blur-[150px]"
		></div>

		<Sidebar />

		<main class="relative z-10 flex-1 overflow-y-auto p-10">
			{@render children()}
		</main>
	</div>
{/if}
