<script lang="ts">
	import { getHistory, type PlaybackHistoryEntry } from '$lib/api';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import ErrorAlert from '$lib/components/ErrorAlert.svelte';

	let history = $state<PlaybackHistoryEntry[]>([]);
	let error = $state('');
	let loading = $state(true);

	$effect(() => {
		getHistory()
			.then((d) => (history = d))
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

	function formatPosition(entry: PlaybackHistoryEntry) {
		if (entry.position == null) return '—';
		const pos = Math.floor(entry.position / 60);
		const total = entry.total ? Math.floor(entry.total / 60) : null;
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
	{:else if history.length === 0}
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
								<th>Progress</th>
								<th>Device</th>
								<th>Time</th>
							</tr>
						</thead>
						<tbody>
							{#each history as entry}
								<tr class="hover">
									<td class="max-w-48 truncate">
										<span class="text-sm" title={entry.episode_url}>
											{episodeName(entry.episode_url)}
										</span>
									</td>
									<td class="max-w-36 truncate">
										<span class="text-sm text-base-content/60" title={entry.podcast_url}>
											{episodeName(entry.podcast_url)}
										</span>
									</td>
									<td class="text-sm text-base-content/60">{formatPosition(entry)}</td>
									<td class="text-sm text-base-content/60">{entry.device_uid || '—'}</td>
									<td class="text-sm text-base-content/60">{formatTimestamp(entry.timestamp)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			</div>
		</div>
		<p class="text-sm text-base-content/40">Showing last {history.length} episodes</p>
	{/if}
</div>
