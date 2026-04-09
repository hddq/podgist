<script lang="ts">
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import { auth } from '$lib/auth.svelte';

	$effect(() => {
		auth.check().then((user) => {
			if (user) {
				goto(`${base}/dashboard`, { replaceState: true });
			} else {
				goto(`${base}/login`, { replaceState: true });
			}
		});
	});
</script>

<div class="flex h-screen items-center justify-center">
	<span class="loading loading-spinner loading-lg text-primary"></span>
</div>
