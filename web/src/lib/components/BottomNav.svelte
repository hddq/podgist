<script lang="ts">
	import { base } from '$app/paths';
	import { page } from '$app/state';

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
</script>

<nav class="fixed bottom-4 left-4 right-4 z-50 lg:hidden flex justify-center pointer-events-none">
	<div
		class="pointer-events-auto flex items-center gap-1 rounded-[28px] border border-zinc-700/40 bg-zinc-900/50 px-3 py-2.5 shadow-[0_8px_32px_rgba(0,0,0,0.45)] backdrop-blur-3xl"
		style="will-change: transform;"
	>
		{#each navItems as item}
			{@const active = isActive(item.href)}
			<a
				href={item.href}
				aria-label={item.label}
				class={[
					'relative flex flex-col items-center gap-1 rounded-2xl px-4 py-3 transition-all duration-300 ease-out select-none',
					active
						? 'bg-zinc-800/90 text-emerald-400 shadow-inner'
						: 'text-zinc-500 hover:text-zinc-300 active:scale-95'
				].join(' ')}
			class:scale-105={active}
			class:scale-100={!active}
			>
				<svg
					xmlns="http://www.w3.org/2000/svg"
					class={[
						'shrink-0 transition-all duration-300',
						active ? 'h-5 w-5' : 'h-5 w-5 opacity-60'
					].join(' ')}
					fill="none"
					viewBox="0 0 24 24"
					stroke="currentColor"
					stroke-width={active ? '2.2' : '1.8'}
				>
					<path stroke-linecap="round" stroke-linejoin="round" d={item.icon} />
				</svg>
			</a>
		{/each}
	</div>
</nav>
