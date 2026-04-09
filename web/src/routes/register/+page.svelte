<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { ApiError, register } from '$lib/api';
	import { auth } from '$lib/auth.svelte';

	let username = $state('');
	let password = $state('');
	let confirmPassword = $state('');
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

		if (password !== confirmPassword) {
			error = 'Passwords do not match.';
			return;
		}

		loading = true;
		try {
			const user = await register(username, password);
			auth.setUser(user);
			goto(`${base}/dashboard`, { replaceState: true });
		} catch (err) {
			if (err instanceof ApiError && err.status === 409) {
				error = 'That username is already taken.';
			} else {
				error = 'Something went wrong. Please try again.';
			}
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Register — Podgist</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center bg-base-100 px-4">
	<div class="card w-full max-w-sm bg-base-200 shadow-xl">
		<div class="card-body gap-4">
			<div class="mb-2 text-center">
				<h1 class="text-3xl font-bold text-primary">🎙 Podgist</h1>
				<p class="mt-1 text-sm text-base-content/60">Create your dashboard account</p>
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
						autocomplete="new-password"
					/>
				</fieldset>

				<fieldset class="fieldset">
					<legend class="fieldset-legend">Confirm Password</legend>
					<input
						type="password"
						class="input w-full"
						placeholder="••••••••"
						bind:value={confirmPassword}
						required
						autocomplete="new-password"
					/>
				</fieldset>

				<button type="submit" class="btn btn-primary mt-2 w-full" disabled={loading}>
					{#if loading}
						<span class="loading loading-spinner loading-sm"></span>
					{/if}
					Create Account
				</button>
			</form>

			<p class="text-center text-sm text-base-content/60">
				Already have an account?
				<a class="link link-primary" href={`${base}/login`}>Sign in</a>
			</p>
		</div>
	</div>
</div>
