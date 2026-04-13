<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { getAccount, logout, type AccountData } from '$lib/api';
	import { auth } from '$lib/auth.svelte';
	import GlassCard from '$lib/components/GlassCard.svelte';
	import LoadingSpinner from '$lib/components/LoadingSpinner.svelte';
	import ErrorAlert from '$lib/components/ErrorAlert.svelte';

	let account = $state<AccountData | null>(null);
	let error = $state('');
	let loading = $state(true);
	let loggingOut = $state(false);

	$effect(() => {
		getAccount()
			.then((d) => (account = d))
			.catch(() => (error = 'Failed to load account data.'))
			.finally(() => (loading = false));
	});

	function formatDate(ts: string) {
		return new Date(ts).toLocaleString();
	}

	async function handleLogout() {
		loggingOut = true;
		try {
			await logout();
			auth.clear();
			goto(`${base}/login`, { replaceState: true });
		} catch {
			loggingOut = false;
		}
	}
</script>

<svelte:head>
	<title>Account — Podgist</title>
</svelte:head>

<div class="flex flex-col gap-6">
	<h1 class="text-2xl font-bold">Account</h1>

	{#if loading}
		<LoadingSpinner />
	{:else if error}
		<ErrorAlert message={error} />
	{:else if account}
		<div class="grid gap-4 lg:grid-cols-2">
			<!-- Account Info -->
			<GlassCard>
				<div class="card-body gap-4">
					<h2 class="card-title">Profile</h2>

					<div class="flex items-center gap-4">
						<div class="avatar placeholder">
							<div class="w-14 rounded-full bg-primary text-primary-content">
								<span class="text-xl font-bold">{account.username[0].toUpperCase()}</span>
							</div>
						</div>
						<div>
							<p class="text-lg font-semibold">{account.username}</p>
							<p class="text-sm text-base-content/50">Member since {formatDate(account.created_at)}</p>
						</div>
					</div>

					<div class="divider my-0"></div>

					<div class="flex flex-col gap-2 text-sm">
						<div class="flex justify-between">
							<span class="text-base-content/60">Username</span>
							<span class="font-mono">{account.username}</span>
						</div>
						<div class="flex justify-between">
							<span class="text-base-content/60">Account created</span>
							<span>{formatDate(account.created_at)}</span>
						</div>
						{#if account.session_expires_at}
							<div class="flex justify-between">
								<span class="text-base-content/60">Session expires</span>
								<span>{formatDate(account.session_expires_at)}</span>
							</div>
						{/if}
					</div>
				</div>
			</GlassCard>

			<!-- Session / Danger zone -->
			<GlassCard>
				<div class="card-body gap-4">
					<h2 class="card-title">Session</h2>
					<p class="text-sm text-base-content/60">
						You are currently signed in. Logging out will end your session and redirect you to the login page.
					</p>
					<div class="card-actions">
						<button class="btn btn-error btn-outline" onclick={handleLogout} disabled={loggingOut}>
							{#if loggingOut}
								<span class="loading loading-spinner loading-sm"></span>
							{/if}
							Sign Out
						</button>
					</div>
				</div>
			</GlassCard>
		</div>
	{/if}
</div>
