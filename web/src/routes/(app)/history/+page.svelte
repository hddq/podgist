<script lang="ts">
	import { getHistory, type EpisodeAction } from '$lib/api';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import ErrorAlert from '$lib/components/ErrorAlert.svelte';

	let actions = $state<EpisodeAction[]>([]);
	let error = $state('');
	let loading = $state(true);

	$effect(() => {
		getHistory()
			.then((d) => (actions = d))
			.catch(() => (error = 'Failed to load playback history.'))
			.finally(() => (loading = false));
	});

	function formatTimestamp(ts: string) {
		return new Date(ts).toLocaleString();
	}

	function episodeName(url: string) {
		try {
			return decodeURIComponent(url.split('/').pop() ?? url);
		} catch {
			return url;
		}
	}

	function formatPosition(action: EpisodeAction) {
		if (action.position == null) return '—';
		const pos = Math.floor(action.position / 60);
		const total = action.total ? Math.floor(action.total / 60) : null;
		return total ? `${pos}m / ${total}m` : `${pos}m`;
	}
</script>

<svelte:head>
	<title>History — Podgist</title>
</svelte:head>

<div class="flex flex-col gap-6">
	<h1 class="text-2xl font-bold">Playback History</h1>

	{#if loading}
		<LoadingSpinner />
	{:else if error}
		<ErrorAlert message={error} />
	{:else if actions.length === 0}
		<div class="card bg-base-200 shadow">
			<div class="card-body">
				<p class="text-base-content/60">No playback history yet.</p>
			</div>
		</div>
	{:else}
		<div class="card bg-base-200 shadow">
			<div class="card-body p-0">
				<div class="overflow-x-auto">
					<table class="table table-sm">
						<thead>
							<tr>
								<th>Episode</th>
								<th>Podcast</th>
								<th>Action</th>
								<th>Progress</th>
								<th>Device</th>
								<th>Time</th>
							</tr>
						</thead>
						<tbody>
							{#each actions as action}
								<tr class="hover">
									<td class="max-w-48 truncate">
										<span class="text-sm" title={action.episode_url}>
											{episodeName(action.episode_url)}
										</span>
									</td>
									<td class="max-w-36 truncate">
										<span class="text-sm text-base-content/60" title={action.podcast_url}>
											{episodeName(action.podcast_url)}
										</span>
									</td>
									<td>
										<span class="badge badge-ghost badge-sm">{action.action}</span>
									</td>
									<td class="text-sm text-base-content/60">{formatPosition(action)}</td>
									<td class="text-sm text-base-content/60">{action.device_uid || '—'}</td>
									<td class="text-sm text-base-content/60">{formatTimestamp(action.timestamp)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		</div>
		<p class="text-sm text-base-content/40">Showing last {actions.length} actions</p>
	{/if}
</div>
