<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { login, ApiError } from '$lib/api';
	import { auth } from '$lib/auth.svelte';

	let username = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		loading = true;
		try {
			const user = await login(username, password);
			auth.setUser(user);
			goto(`${base}/dashboard`, { replaceState: true });
		} catch (err) {
			if (err instanceof ApiError && err.status === 401) {
				error = 'Invalid username or password.';
			} else {
				error = 'Something went wrong. Please try again.';
			}
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Login — Podgist</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-base-100 px-4">
	<div class="card w-full max-w-sm bg-base-200 shadow-xl">
		<div class="card-body gap-4">
			<div class="mb-2 text-center">
				<h1 class="text-3xl font-bold text-primary">🎙 Podgist</h1>
				<p class="mt-1 text-sm text-base-content/60">Sign in to your dashboard</p>
			</div>

			{#if error}
				<div role="alert" class="alert alert-error py-2 text-sm">
					<span>{error}</span>
				</div>
			{/if}

			<form onsubmit={handleSubmit} class="flex flex-col gap-3">
				<fieldset class="fieldset">
					<legend class="fieldset-legend">Username</legend>
					<input
						type="text"
						class="input w-full"
						placeholder="username"
						bind:value={username}
						required
						autocomplete="username"
					/>
				</fieldset>

				<fieldset class="fieldset">
					<legend class="fieldset-legend">Password</legend>
					<input
						type="password"
						class="input w-full"
						placeholder="••••••••"
						bind:value={password}
						required
						autocomplete="current-password"
					/>
				</fieldset>

				<button type="submit" class="btn btn-primary mt-2 w-full" disabled={loading}>
					{#if loading}
						<span class="loading loading-spinner loading-sm"></span>
					{/if}
					Sign In
				</button>
			</form>
		</div>
	</div>
</div>
