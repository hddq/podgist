<script lang="ts">
	import { getDashboard, type DashboardData } from '$lib/api';
	import StatCard from '$lib/components/StatCard.svelte';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import ErrorAlert from '$lib/components/ErrorAlert.svelte';

	let data = $state<DashboardData | null>(null);
	let error = $state('');
	let loading = $state(true);

	$effect(() => {
		getDashboard()
			.then((d) => (data = d))
			.catch(() => (error = 'Failed to load dashboard data.'))
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
	<title>Dashboard — Podgist</title>
</svelte:head>

<div class="flex flex-col gap-6">
	<h1 class="text-2xl font-bold">Dashboard</h1>

	{#if loading}
		<LoadingSpinner />
	{:else if error}
		<ErrorAlert message={error} />
	{:else if data}
		<!-- Stats -->
		<div class="stats stats-vertical w-full shadow lg:stats-horizontal bg-base-200">
			<StatCard title="Subscriptions" value={data.subscription_count} desc="Unique podcasts" />
			<StatCard title="Devices" value={data.device_count} desc="Registered devices" />
			<StatCard title="Episode Actions" value={data.episode_action_count} desc="Total recorded actions" />
		</div>

		<!-- Recent Activity -->
		<div class="card bg-base-200 shadow">
			<div class="card-body">
				<h2 class="card-title text-lg">Recent Activity</h2>
				{#if data.recent_actions.length === 0}
					<p class="text-base-content/60">No recent activity yet.</p>
				{:else}
					<div class="overflow-x-auto">
						<table class="table table-sm">
							<thead>
								<tr>
									<th>Episode</th>
									<th>Action</th>
									<th>Device</th>
									<th>Time</th>
								</tr>
							</thead>
							<tbody>
								{#each data.recent_actions as action (action.timestamp + ':' + action.episode_url + ':' + action.action)}
									<tr>
										<td class="max-w-xs">
											<div class="flex flex-col">
												<span class="truncate text-sm font-medium" title={action.episode_url}>
													{action.episode_title || episodeName(action.episode_url)}
												</span>
												<span class="truncate text-xs text-base-content/60" title={action.podcast_url}>
													{action.podcast_title || podcastName(action.podcast_url)}
												</span>
											</div>
										</td>
										<td>
											<span class="badge badge-ghost badge-sm">{action.action}</span>
										</td>
										<td class="text-sm text-base-content/60">{action.device_uid || '—'}</td>
										<td class="text-sm text-base-content/60">{formatTimestamp(action.timestamp)}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>
