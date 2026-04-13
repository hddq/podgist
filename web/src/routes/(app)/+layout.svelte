<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { auth } from '$lib/auth.svelte';
	import { logout } from '$lib/api';
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

	async function handleLogout() {
		await logout();
		auth.clear();
		goto(`${base}/login`, { replaceState: true });
	}
</script>

{#if checking}
	<div class="flex h-screen items-center justify-center">
		<span class="loading loading-spinner loading-lg text-primary"></span>
	</div>
{:else}
	<div class="relative min-h-screen overflow-hidden bg-black text-zinc-100 font-sans">
		<div
			class="pointer-events-none absolute left-[10%] top-[-10%] h-[500px] w-[500px] rounded-full bg-emerald-600/10 blur-[120px]"
		></div>
		<div
			class="pointer-events-none absolute bottom-[-10%] right-[10%] h-[600px] w-[600px] rounded-full bg-indigo-600/10 blur-[150px]"
		></div>

		<div class="relative z-10 drawer h-dvh overflow-hidden lg:drawer-open">
			<input id="main-drawer" type="checkbox" class="drawer-toggle" />

			<!-- Drawer content -->
			<div class="drawer-content flex h-dvh min-h-0 flex-col overflow-hidden">
				<!-- Mobile-only navbar -->
				<div class="navbar border-b border-zinc-800 bg-zinc-900/60 backdrop-blur-md lg:hidden">
					<div class="flex-none">
						<label for="main-drawer" class="btn btn-square btn-ghost text-zinc-100" aria-label="Open menu">
							<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" class="inline-block h-5 w-5 stroke-current">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"></path>
							</svg>
						</label>
					</div>
					<div class="flex-1">
						<span class="text-lg font-semibold">🎙 Podgist</span>
					</div>
				</div>

				<!-- Page content -->
				<main class="flex-1 overflow-y-auto p-4 lg:p-8">
					{@render children()}
				</main>
			</div>

			<!-- Drawer side -->
			<div class="drawer-side h-dvh">
				<label for="main-drawer" aria-label="Close menu" class="drawer-overlay"></label>
				<div class="flex h-full w-fit min-w-max flex-col overflow-hidden border-r border-zinc-800 bg-zinc-950/60 backdrop-blur-md">
					<Sidebar />
					<div class="mt-auto p-4">
						{#if auth.user}
							<p class="mb-2 truncate text-sm text-zinc-400">{auth.user.username}</p>
						{/if}
						<button class="btn btn-ghost btn-sm w-full text-zinc-100" onclick={handleLogout}>Logout</button>
					</div>
				</div>
			</div>
		</div>
	</div>
{/if}
