<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { page } from '$app/state';
	import { logout } from '$lib/api';
	import { auth } from '$lib/auth.svelte';

	const navItems = [
		{ href: `${base}/dashboard`, label: 'Dashboard', icon: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6' },
		{ href: `${base}/history`, label: 'History', icon: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z' },
		{ href: `${base}/subscriptions`, label: 'Subscriptions', icon: 'M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10' },
		{ href: `${base}/devices`, label: 'Devices', icon: 'M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z' },
		{ href: `${base}/account`, label: 'Account', icon: 'M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z' }
	];

	function isActive(href: string) {
		return page.url.pathname === href || page.url.pathname.startsWith(href + '/');
	}

	async function handleLogout() {
		await logout();
		auth.clear();
		goto(`${base}/login`, { replaceState: true });
	}
</script>

<aside class="z-20 hidden h-screen w-64 flex-col border-r border-zinc-800/50 bg-black p-4 lg:fixed lg:left-0 lg:top-0 lg:flex">
	<div class="mb-8 flex items-center gap-3 px-2">
		<div class="flex h-8 w-8 items-center justify-center rounded-lg bg-zinc-800 text-zinc-200">🎧</div>
		<span class="text-lg font-bold tracking-wide text-zinc-100">Podgist</span>
	</div>

	<nav class="flex-1 space-y-2">
		{#each navItems as item}
			<a
				href={item.href}
				class={`group flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors ${
					isActive(item.href)
						? 'border border-zinc-700 bg-zinc-900 text-zinc-100'
						: 'border border-transparent text-zinc-400 hover:border-zinc-800 hover:bg-zinc-900/60 hover:text-zinc-200'
				}`}
			>
				<svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={item.icon} />
				</svg>
				<span>{item.label}</span>
			</a>
		{/each}
	</nav>

	<div class="mt-auto border-t border-zinc-800/50 px-2 pt-4">
		{#if auth.user}
			<p class="mb-3 truncate text-xs uppercase tracking-wide text-zinc-400">{auth.user.username}</p>
		{/if}
		<button class="btn btn-ghost btn-sm w-full justify-start text-zinc-200 hover:bg-zinc-800/60" onclick={handleLogout}>
			Logout
		</button>
	</div>
</aside>
