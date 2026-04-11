<script lang="ts">
	import { getSubscriptions, type Subscription } from '$lib/api';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import ErrorAlert from '$lib/components/ErrorAlert.svelte';

	let subs = $state<Subscription[]>([]);
	let error = $state('');
	let loading = $state(true);

	$effect(() => {
		getSubscriptions()
			.then((d) => (subs = d))
			.catch(() => (error = 'Failed to load subscriptions.'))
			.finally(() => (loading = false));
	});

	function podcastName(url: string) {
		try {
			const u = new URL(url);
			return u.hostname + u.pathname;
		} catch {
			return url;
		}
	}
</script>

<svelte:head>
	<title>Subscriptions — Podgist</title>
</svelte:head>

<div class="flex flex-col gap-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold">Subscriptions</h1>
		{#if !loading && !error}
			<span class="badge badge-neutral">{subs.length} podcasts</span>
		{/if}
	</div>

	{#if loading}
		<LoadingSpinner />
	{:else if error}
		<ErrorAlert message={error} />
	{:else if subs.length === 0}
		<div class="card bg-base-200 shadow">
			<div class="card-body">
				<p class="text-base-content/60">No subscriptions yet. Add podcasts from your client app.</p>
			</div>
		</div>
	{:else}
		<div class="flex flex-col gap-3">
			{#each subs as sub (sub.podcast_url)}
				<div class="card bg-base-200 shadow-sm">
					<div class="card-body gap-2 py-4">
						<a
							href={sub.podcast_url}
							target="_blank"
							rel="noopener noreferrer"
							class="link link-hover text-sm font-medium"
							title={sub.podcast_url}
						>
							{sub.podcast_title || podcastName(sub.podcast_url)}
						</a>
						<div class="flex flex-wrap gap-1">
							{#each sub.devices as device (device)}
								<span class="badge badge-ghost badge-sm">{device}</span>
							{/each}
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
