<script lang="ts">
	import { getDevices, type Device } from '$lib/api';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import ErrorAlert from '$lib/components/ErrorAlert.svelte';

	let devices = $state<Device[]>([]);
	let error = $state('');
	let loading = $state(true);

	$effect(() => {
		getDevices()
			.then((d) => (devices = d))
			.catch(() => (error = 'Failed to load devices.'))
			.finally(() => (loading = false));
	});

	function formatDate(ts: string) {
		return new Date(ts).toLocaleDateString();
	}

	const deviceTypeColors: Record<string, string> = {
		mobile: 'badge-primary',
		desktop: 'badge-secondary',
		laptop: 'badge-accent',
		server: 'badge-info',
		other: 'badge-ghost'
	};

	function deviceBadge(type: string) {
		return deviceTypeColors[type] ?? 'badge-ghost';
	}
</script>

<svelte:head>
	<title>Devices — Podgist</title>
</svelte:head>

<div class="flex flex-col gap-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold">Devices</h1>
		{#if !loading && !error}
			<span class="badge badge-neutral">{devices.length} devices</span>
		{/if}
	</div>

	{#if loading}
		<LoadingSpinner />
	{:else if error}
		<ErrorAlert message={error} />
	{:else if devices.length === 0}
		<div class="card bg-base-200 shadow">
			<div class="card-body">
				<p class="text-base-content/60">No devices registered yet. Connect a podcast client to get started.</p>
			</div>
		</div>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
			{#each devices as device}
				<div class="card bg-base-200 shadow-sm">
					<div class="card-body gap-3">
						<div class="flex items-start justify-between gap-2">
							<div>
								<h3 class="font-semibold">{device.caption || device.uid}</h3>
								{#if device.caption}
									<p class="text-xs text-base-content/50">{device.uid}</p>
								{/if}
							</div>
							<span class="badge badge-sm {deviceBadge(device.type)}">{device.type}</span>
						</div>
						<div class="flex items-center gap-2 text-sm text-base-content/60">
							<svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
								<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
							</svg>
							{device.subscription_count} subscription{device.subscription_count !== 1 ? 's' : ''}
						</div>
						<div class="text-xs text-base-content/40">
							Added {formatDate(device.created_at)}
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
