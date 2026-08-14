<script lang="ts">
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { appState } from '$lib/state/app.svelte';

	let { children }: { children: Snippet } = $props();
	let menuOpen = $state(false);

	const navigation = [
		{ href: '/', label: 'Wake center', icon: '⌁' },
		{ href: '/devices', label: 'Devices', icon: '▦' },
		{ href: '/groups', label: 'Groups', icon: '◇' },
		{ href: '/sites', label: 'Sites', icon: '⌖' },
		{ href: '/history', label: 'History', icon: '↗' },
		{ href: '/config', label: 'Config & data', icon: '⚙' }
	];

	function isActive(href: string) {
		return href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(href);
	}
</script>

<svelte:head>
	<title>WOL Control Room</title>
	<meta name="description" content="A local-first Wake-on-LAN control room" />
</svelte:head>

<div class="app-shell">
	<button class:open={menuOpen} class="mobile-scrim" aria-label="Close navigation" onclick={() => (menuOpen = false)}></button>
	<aside class:open={menuOpen} class="sidebar">
		<div class="brand-lockup">
			<div class="brand-mark" aria-hidden="true"><span></span><span></span><span></span></div>
			<div>
				<div class="brand-name">WOL</div>
				<div class="brand-caption">CONTROL ROOM</div>
			</div>
		</div>

		<div class="sidebar-label">Operations</div>
		<nav aria-label="Primary navigation">
			{#each navigation as item}
				<a class:active={isActive(item.href)} href={item.href} aria-current={isActive(item.href) ? 'page' : undefined} onclick={() => (menuOpen = false)}>
					<span class="nav-icon" aria-hidden="true">{item.icon}</span>
					<span>{item.label}</span>
				</a>
			{/each}
		</nav>

		<div class="sidebar-footer">
			<div class="connection-line">
				<span class:offline={!appState.connected} class="status-dot"></span>
				<span>{appState.connected ? 'Server connected' : 'Server offline'}</span>
			</div>
			<div class="mono muted">v{appState.bootstrap?.version ?? '0.1.0'}</div>
		</div>
	</aside>

	<section class="main-column">
		<header class="topbar">
			<button class="icon-button menu-button" aria-label="Open navigation" onclick={() => (menuOpen = true)}>☰</button>
			<div class="breadcrumb"><span class="muted">WOL</span><span>/</span><strong>{page.url.pathname === '/' ? 'Wake center' : page.url.pathname.slice(1).split('/')[0]}</strong></div>
			<div class="topbar-actions">
				<div class="server-chip">
					<span class:offline={!appState.connected} class="status-dot"></span>
					<span>{appState.connected ? 'Local server ready' : 'Waiting for server'}</span>
				</div>
				<a class="avatar" href="/config" aria-label="Open settings">AK</a>
			</div>
		</header>

		<main class="page-content">
			{#if appState.error}
				<div class="global-error" role="alert">
					<div><strong>Server connection needs attention</strong><span>{appState.error}</span></div>
					<button class="button button-ghost" onclick={() => location.reload()}>Retry</button>
				</div>
			{/if}
			{@render children()}
		</main>
	</section>
</div>

<style>
	.app-shell { min-height: 100vh; display: flex; background: var(--ink); color: var(--text); }
	.sidebar { width: 248px; flex: 0 0 248px; min-height: 100vh; display: flex; flex-direction: column; padding: 28px 18px 20px; background: linear-gradient(180deg, #101a25 0%, #0c141d 100%); border-right: 1px solid var(--line); position: relative; z-index: 20; }
	.brand-lockup { display: flex; align-items: center; gap: 12px; padding: 0 10px 34px; }
	.brand-mark { width: 32px; height: 32px; display: flex; align-items: flex-end; gap: 3px; padding: 6px; border: 1px solid rgba(56,217,169,.38); border-radius: 10px; background: rgba(56,217,169,.08); box-shadow: inset 0 0 18px rgba(56,217,169,.08); }
	.brand-mark span { width: 4px; border-radius: 3px; background: var(--signal); box-shadow: 0 0 12px rgba(56,217,169,.7); }
	.brand-mark span:nth-child(1) { height: 8px; opacity: .55; }
	.brand-mark span:nth-child(2) { height: 14px; opacity: .8; }
	.brand-mark span:nth-child(3) { height: 20px; }
	.brand-name { font: 700 18px/1 var(--display); letter-spacing: .08em; }
	.brand-caption { color: var(--muted); font: 600 9px/1.2 var(--mono); letter-spacing: .16em; margin-top: 5px; }
	.sidebar-label { padding: 0 12px 10px; color: var(--muted); font: 600 10px/1 var(--mono); letter-spacing: .14em; text-transform: uppercase; }
	nav { display: grid; gap: 4px; }
	nav a { display: flex; align-items: center; gap: 12px; min-height: 44px; padding: 0 12px; color: #9db0bf; text-decoration: none; border: 1px solid transparent; border-radius: 12px; font-size: 13px; font-weight: 600; transition: background 160ms ease, color 160ms ease, border-color 160ms ease; }
	nav a:hover { background: rgba(255,255,255,.045); color: var(--text); }
	nav a.active { color: var(--text); background: rgba(56,217,169,.09); border-color: rgba(56,217,169,.2); box-shadow: inset 3px 0 0 var(--signal); }
	.nav-icon { width: 20px; color: var(--signal); font: 20px/1 var(--mono); text-align: center; }
	.sidebar-footer { margin-top: auto; display: grid; gap: 12px; padding: 14px 12px 0; border-top: 1px solid var(--line); }
	.connection-line { display: flex; align-items: center; gap: 8px; color: #b8c6d0; font-size: 11px; }
	.status-dot { display: inline-block; width: 7px; height: 7px; border-radius: 50%; background: var(--signal); box-shadow: 0 0 0 4px rgba(56,217,169,.12), 0 0 12px rgba(56,217,169,.55); }
	.status-dot.offline { background: var(--danger); box-shadow: 0 0 0 4px rgba(245,108,108,.12); }
	.main-column { flex: 1; min-width: 0; }
	.topbar { height: 72px; display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 0 34px; border-bottom: 1px solid var(--line); background: rgba(11,17,24,.82); backdrop-filter: blur(14px); position: sticky; top: 0; z-index: 10; }
	.breadcrumb { display: flex; align-items: center; gap: 10px; color: var(--text); font-size: 13px; text-transform: capitalize; }
	.topbar-actions { display: flex; align-items: center; gap: 16px; }
	.server-chip { display: flex; align-items: center; gap: 9px; color: #a9bbc7; font: 11px var(--mono); }
	.avatar { width: 32px; height: 32px; display: grid; place-items: center; color: var(--ink); background: var(--signal); border-radius: 10px; text-decoration: none; font: 700 11px var(--mono); }
	.page-content { max-width: 1480px; margin: 0 auto; padding: 42px 44px 72px; }
	.global-error { display: flex; align-items: center; justify-content: space-between; gap: 20px; margin-bottom: 24px; padding: 14px 16px; border: 1px solid rgba(245,108,108,.3); border-radius: 14px; background: rgba(245,108,108,.08); color: #ffd6d6; font-size: 13px; }
	.global-error div { display: grid; gap: 4px; }
	.global-error span { color: #efa8a8; font-size: 12px; }
	.icon-button, .menu-button { display: none; }
	@media (max-width: 820px) {
		.sidebar { position: fixed; left: 0; top: 0; transform: translateX(-100%); transition: transform 180ms ease; box-shadow: 20px 0 60px rgba(0,0,0,.32); }
		.sidebar.open { transform: translateX(0); }
		.mobile-scrim { display: none; position: fixed; inset: 0; z-index: 19; width: 100%; height: 100%; padding: 0; border: 0; background: rgba(2,6,10,.7); appearance: none; }
		.mobile-scrim.open { display: block; }
		.menu-button { display: grid; }
		.topbar { height: 64px; padding: 0 18px; }
		.breadcrumb { flex: 1; }
		.server-chip { display: none; }
		.page-content { padding: 28px 18px 56px; }
	}
	@media (prefers-reduced-motion: reduce) {
		.sidebar, nav a { transition: none; }
	}
</style>
