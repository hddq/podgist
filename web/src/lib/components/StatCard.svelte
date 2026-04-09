<script lang="ts">
	let { title, value, desc = '' }: { title: string; value: string | number; desc?: string } = $props();
</script>

<div class="stat">
	<div class="stat-title">{title}</div>
	<div class="stat-value text-primary">{value}</div>
	{#if desc}
		<div class="stat-desc">{desc}</div>
	{/if}
</div>
