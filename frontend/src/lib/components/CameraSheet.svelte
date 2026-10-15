<script lang="ts">
	// The receipt camera on a phone: a live viewfinder with a shutter and a way
	// into the camera roll, which the phone's own camera (<input capture>) lacks.
	// Opened and closed through lib/capture.svelte.ts; whatever it produces goes
	// to takeFile, which closes it.
	import { onMount } from 'svelte';
	import { closeCamera, openLibrary, openSystemCamera, takeFile } from '$lib/capture.svelte';

	let video: HTMLVideoElement;
	let status = $state<'starting' | 'live' | 'failed'>('starting');
	let failure = $state('');
	let shooting = $state(false);

	onMount(() => {
		let stream: MediaStream | null = null;
		let cancelled = false;

		navigator.mediaDevices
			.getUserMedia({
				audio: false,
				// As sharp as the back camera goes: receipt print is small.
				video: { facingMode: { ideal: 'environment' }, width: { ideal: 3840 }, height: { ideal: 2160 } }
			})
			.then(async (s) => {
				stream = s;
				if (cancelled) return;
				video.srcObject = s;
				await video.play();
				if (!cancelled) status = 'live';
			})
			.catch((e) => {
				if (cancelled) return;
				status = 'failed';
				failure =
					e instanceof DOMException && e.name === 'NotAllowedError'
						? 'Camera access is off for this app.'
						: "The camera didn't start.";
			});

		const onKey = (e: KeyboardEvent) => {
			if (e.key === 'Escape') closeCamera();
		};
		window.addEventListener('keydown', onKey);

		return () => {
			cancelled = true;
			window.removeEventListener('keydown', onKey);
			stream?.getTracks().forEach((t) => t.stop());
		};
	});

	function shoot() {
		if (status !== 'live' || shooting || !video.videoWidth) return;
		shooting = true;
		const canvas = document.createElement('canvas');
		canvas.width = video.videoWidth;
		canvas.height = video.videoHeight;
		canvas.getContext('2d')?.drawImage(video, 0, 0);
		canvas.toBlob(
			(blob) => {
				shooting = false;
				if (blob) takeFile(new File([blob], 'receipt.jpg', { type: 'image/jpeg' }));
			},
			'image/jpeg',
			0.92
		);
	}
</script>

<div class="sheet" role="dialog" aria-modal="true" aria-label="Receipt camera">
	<button class="close" onclick={closeCamera}>Cancel</button>

	<div class="view">
		<!-- svelte-ignore a11y_media_has_caption -->
		<video bind:this={video} playsinline muted autoplay class:hidden={status !== 'live'}></video>
		{#if status === 'starting'}
			<p class="message">Starting camera…</p>
		{:else if status === 'failed'}
			<div class="message">
				<p>{failure}</p>
				<button class="fallback" onclick={openSystemCamera}>Use the phone's camera</button>
			</div>
		{/if}
	</div>

	<div class="controls">
		<button class="roll" onclick={openLibrary} aria-label="Choose from camera roll">
			<svg viewBox="0 0 24 24" aria-hidden="true"
				><rect x="3" y="4" width="18" height="16" rx="2" /><circle cx="9" cy="10" r="1.5" /><path
					d="m21 16-5-5-9 9"
				/></svg
			>
			<span>Photos</span>
		</button>
		<button
			class="shutter"
			onclick={shoot}
			disabled={status !== 'live' || shooting}
			aria-label="Take photo"
		></button>
		<span class="spacer"></span>
	</div>
</div>

<style>
	.sheet {
		position: fixed;
		inset: 0;
		z-index: 300;
		display: flex;
		flex-direction: column;
		background: #000;
		color: #fff;
		padding: env(safe-area-inset-top) 0 env(safe-area-inset-bottom);
	}

	.close {
		align-self: flex-start;
		margin: 0.5rem 0.75rem;
		padding: 0.5rem 0.75rem;
		border: none;
		background: none;
		color: #fff;
		font: inherit;
		font-size: 1rem;
		cursor: pointer;
	}

	.view {
		flex: 1;
		min-height: 0;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	/* contain, not cover: what's on screen is what gets sent. */
	video {
		width: 100%;
		height: 100%;
		object-fit: contain;
	}

	video.hidden {
		display: none;
	}

	.message {
		margin: 0 1.5rem;
		text-align: center;
		color: rgb(255 255 255 / 0.8);
	}

	.fallback {
		margin-top: 0.75rem;
		padding: 0.625rem 1rem;
		border: 1px solid rgb(255 255 255 / 0.6);
		border-radius: 999px;
		background: none;
		color: #fff;
		font: inherit;
		cursor: pointer;
	}

	.controls {
		display: grid;
		grid-template-columns: 1fr auto 1fr;
		align-items: center;
		padding: 1.25rem 1.5rem 1.5rem;
	}

	.roll {
		justify-self: start;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.25rem;
		padding: 0.25rem;
		border: none;
		background: none;
		color: #fff;
		font: inherit;
		font-size: 0.75rem;
		cursor: pointer;
	}

	.roll svg {
		width: 44px;
		height: 44px;
		padding: 9px;
		border-radius: 10px;
		background: rgb(255 255 255 / 0.15);
		fill: none;
		stroke: #fff;
		stroke-width: 1.75;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	.shutter {
		width: 72px;
		height: 72px;
		border-radius: 50%;
		border: 4px solid #fff;
		background: #fff;
		box-shadow: inset 0 0 0 3px #000;
		cursor: pointer;
	}

	.shutter:active:not(:disabled) {
		transform: scale(0.94);
	}

	.shutter:disabled {
		opacity: 0.4;
		cursor: default;
	}
</style>
