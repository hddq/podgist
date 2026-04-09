<script lang="ts">
	import './layout.css';

	let { children } = $props();
</script>

<svelte:head>
	<title>Podgist</title>
</svelte:head>

{@render children()}
