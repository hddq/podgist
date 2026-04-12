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
	<div class="drawer h-dvh overflow-hidden lg:drawer-open">
		<input id="main-drawer" type="checkbox" class="drawer-toggle" />

		<!-- Drawer content -->
		<div class="drawer-content flex h-dvh min-h-0 flex-col overflow-hidden">
			<!-- Mobile-only navbar -->
			<div class="navbar border-b border-base-300 bg-base-200 lg:hidden">
				<div class="flex-none">
					<label for="main-drawer" class="btn btn-square btn-ghost" aria-label="Open menu">
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
			<div class="flex h-full w-fit min-w-max flex-col overflow-hidden bg-base-200">
				<Sidebar />
				<div class="mt-auto p-4">
					{#if auth.user}
						<p class="mb-2 truncate text-sm text-base-content/60">{auth.user.username}</p>
					{/if}
					<button class="btn btn-ghost btn-sm w-full" onclick={handleLogout}>Logout</button>
				</div>
			</div>
		</div>
	</div>
{/if}
