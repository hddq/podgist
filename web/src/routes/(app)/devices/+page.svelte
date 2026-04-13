<script lang="ts">
	import { onMount } from 'svelte';
	import { getDevices, getSyncDevices, updateSyncDevices, type Device, type SyncStatus } from '$lib/api';
	import GlassCard from '$lib/components/GlassCard.svelte';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import ErrorAlert from '$lib/components/ErrorAlert.svelte';

	let devices = $state<Device[]>([]);
	let syncStatus = $state<SyncStatus>({ synchronized: [], 'not-synchronized': [] });
	let selectedForSync = $state<string[]>([]);
	let error = $state('');
	let actionError = $state('');
	let loading = $state(true);
	let saving = $state(false);

	onMount(() => {
		void loadData();
	});

	async function loadData() {
		error = '';
		loading = true;
		try {
			const [d, s] = await Promise.all([getDevices(), getSyncDevices()]);
			devices = d;
			syncStatus = s;
		} catch {
			error = 'Failed to load devices and sync status.';
		} finally {
			loading = false;
		}
	}

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

	function deviceLabel(uid: string) {
		const device = devices.find((d) => d.uid === uid);
		if (!device) {
			return uid;
		}
		return device.caption ? `${device.caption} (${uid})` : uid;
	}

	function syncedDeviceUIDs() {
		return Array.from(new Set(syncStatus.synchronized.flat()));
	}

	function toggleSyncSelection(uid: string, checked: boolean) {
		if (checked) {
			if (!selectedForSync.includes(uid)) {
				selectedForSync = [...selectedForSync, uid];
			}
			return;
		}
		selectedForSync = selectedForSync.filter((currentUID) => currentUID !== uid);
	}

	async function createOrReplaceSyncGroup() {
		if (saving || selectedForSync.length < 2) {
			return;
		}

		actionError = '';
		saving = true;
		try {
			syncStatus = await updateSyncDevices({
				synchronize: [selectedForSync],
				'stop-synchronize': []
			});
			devices = await getDevices();
			selectedForSync = [];
		} catch (err) {
			actionError = err instanceof Error && err.message ? err.message : 'Failed to update sync group.';
		} finally {
			saving = false;
		}
	}

	async function stopSync(uid: string) {
		if (saving) {
			return;
		}

		actionError = '';
		saving = true;
		try {
			syncStatus = await updateSyncDevices({
				synchronize: [],
				'stop-synchronize': [uid]
			});
			devices = await getDevices();
			selectedForSync = selectedForSync.filter((currentUID) => currentUID !== uid);
		} catch (err) {
			actionError = err instanceof Error && err.message ? err.message : 'Failed to stop synchronization.';
		} finally {
			saving = false;
		}
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
		<GlassCard>
			<div class="card-body">
				<p class="text-base-content/60">No devices registered yet. Connect a podcast client to get started.</p>
			</div>
		</GlassCard>
	{:else}
		<GlassCard class="shadow-xl">
			<div class="card-body gap-4">
				<div class="flex items-center justify-between gap-3">
					<h2 class="text-lg font-semibold">Synchronization</h2>
					{#if saving}
						<span class="loading loading-spinner loading-sm text-primary"></span>
					{/if}
				</div>

				{#if actionError}
					<ErrorAlert message={actionError} />
				{/if}

				{#if devices.length < 2}
					<p class="text-sm text-base-content/60">Add at least two devices to manage synchronization groups.</p>
				{:else}
					<div class="grid gap-4 lg:grid-cols-2">
						<div class="rounded-xl border border-zinc-800 bg-black/20 p-4">
							<h3 class="font-medium">Current sync groups</h3>
							{#if syncStatus.synchronized.length === 0}
								<p class="mt-2 text-sm text-base-content/60">No synchronized device groups yet.</p>
							{:else}
								<div class="mt-3 space-y-2">
									{#each syncStatus.synchronized as group, i (group.join('|') + '-' + i)}
										<div class="rounded-lg border border-zinc-800 bg-zinc-900/50 p-3">
											<p class="mb-2 text-xs uppercase tracking-wide text-base-content/50">Group {i + 1}</p>
											<div class="flex flex-wrap gap-2">
												{#each group as uid (uid)}
													<span class="badge badge-outline">{deviceLabel(uid)}</span>
												{/each}
											</div>
										</div>
									{/each}
								</div>
							{/if}

							<p class="mt-4 text-xs text-base-content/50">
								Not synchronized:
								{syncStatus['not-synchronized'].length === 0
									? 'none'
									: syncStatus['not-synchronized'].map((uid) => deviceLabel(uid)).join(', ')}
							</p>
						</div>

						<div class="rounded-xl border border-zinc-800 bg-black/20 p-4">
							<h3 class="font-medium">Group editor</h3>
							<p class="mt-1 text-sm text-base-content/60">Select two or more devices to create or replace a sync group.</p>
							<div class="mt-3 space-y-1">
								{#each devices as device (device.uid)}
									<label class="label cursor-pointer justify-start gap-3 rounded-md px-2 py-1 hover:bg-zinc-800/50">
										<input
											type="checkbox"
											class="checkbox checkbox-sm"
											checked={selectedForSync.includes(device.uid)}
											onchange={(event) =>
												toggleSyncSelection(device.uid, (event.currentTarget as HTMLInputElement).checked)}
											disabled={saving}
										/>
										<span class="label-text">{device.caption || device.uid}</span>
										{#if device.caption}
											<span class="text-xs text-base-content/50">{device.uid}</span>
										{/if}
									</label>
								{/each}
							</div>
							<button
								class="btn btn-primary btn-sm mt-4"
								onclick={createOrReplaceSyncGroup}
								disabled={saving || selectedForSync.length < 2}
							>
								Create / replace group
							</button>
						</div>
					</div>

					<div class="rounded-xl border border-zinc-800 bg-black/20 p-4">
						<h3 class="font-medium">Stop sync per device</h3>
						{#if syncedDeviceUIDs().length === 0}
							<p class="mt-2 text-sm text-base-content/60">No devices are currently synchronized.</p>
						{:else}
							<div class="mt-3 flex flex-wrap gap-2">
								{#each syncedDeviceUIDs() as uid (uid)}
									<button class="btn btn-outline btn-xs" onclick={() => stopSync(uid)} disabled={saving}>
										Unsync {deviceLabel(uid)}
									</button>
								{/each}
							</div>
						{/if}
					</div>
				{/if}
			</div>
		</GlassCard>

		<div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
			{#each devices as device (device.uid)}
				<GlassCard class="shadow-lg">
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
				</GlassCard>
			{/each}
		</div>
	{/if}
</div>
