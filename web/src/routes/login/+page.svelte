<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { login, ApiError } from '$lib/api';
	import { auth } from '$lib/auth.svelte';
	import AuthLayout from '$lib/components/AuthLayout.svelte';

	let username = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	$effect(() => {
		auth.check().then((user) => {
			if (user) {
				goto(`${base}/dashboard`, { replaceState: true });
			}
		});
	});

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

<AuthLayout title="Login" subtitle="Sign in to your dashboard" {error}>
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

	{#snippet footer()}
		<p class="text-center text-sm text-base-content/60">
			Need an account?
			<a class="link link-primary" href={`${base}/register`}>Create one</a>
		</p>
	{/snippet}
</AuthLayout>
