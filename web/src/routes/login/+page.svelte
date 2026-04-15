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
	<form onsubmit={handleSubmit} class="flex flex-col gap-4">
		<fieldset class="fieldset gap-1">
			<legend class="fieldset-legend text-zinc-300">Username</legend>
			<input
				type="text"
				class="input w-full border-zinc-700 bg-zinc-950/70 text-zinc-100 placeholder:text-zinc-500"
				placeholder="username"
				bind:value={username}
				required
				autocomplete="username"
			/>
		</fieldset>

		<fieldset class="fieldset gap-1">
			<legend class="fieldset-legend text-zinc-300">Password</legend>
			<input
				type="password"
				class="input w-full border-zinc-700 bg-zinc-950/70 text-zinc-100 placeholder:text-zinc-500"
				placeholder="••••••••"
				bind:value={password}
				required
				autocomplete="current-password"
			/>
		</fieldset>

		<button
			type="submit"
			class="btn mt-2 w-full border-emerald-500/40 bg-emerald-500 text-black hover:bg-emerald-400"
			disabled={loading}
		>
			{#if loading}
				<span class="loading loading-spinner loading-sm"></span>
			{/if}
			Sign In
		</button>
	</form>

	{#snippet footer()}
		<p class="text-center text-sm text-zinc-400">
			Need an account?
			<a class="font-medium text-emerald-400 transition-colors hover:text-emerald-300" href={`${base}/register`}
				>Create one</a
			>
		</p>
	{/snippet}
</AuthLayout>
