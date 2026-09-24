// Receipt photos are shrunk and re-encoded in the browser before upload.
//
// That does three jobs at once. A phone photo is 4–12 MB and the model gains
// nothing past 2576px on the long edge, so the upload gets several times
// smaller. Re-encoding through a canvas turns an iPhone HEIC into a JPEG, the
// one format every part of the pipeline takes. And the canvas drops EXIF,
// including the GPS position the camera wrote, before the photo is stored.

/** Longest edge the reading model makes use of. */
export const MAX_EDGE = 2576;

/** Scales width × height down (never up) so the longer side is at most max. */
export function fitWithin(width: number, height: number, max: number): { width: number; height: number } {
	const longest = Math.max(width, height);
	if (longest <= max) return { width, height };
	const scale = max / longest;
	return {
		width: Math.max(1, Math.round(width * scale)),
		height: Math.max(1, Math.round(height * scale))
	};
}

/**
 * Decodes an image file and returns it as a JPEG no larger than maxEdge on its
 * long side, upright. Throws if the browser cannot decode the file — some
 * desktop browsers cannot read HEIC.
 */
export async function downscale(file: Blob, maxEdge = MAX_EDGE, quality = 0.85): Promise<Blob> {
	let bitmap: ImageBitmap;
	try {
		// from-image applies the EXIF rotation, so a portrait photo stays upright
		// once the EXIF is gone.
		bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' });
	} catch {
		throw new Error("Couldn't read that image. Try a JPEG or PNG, or take the photo again.");
	}

	const { width, height } = fitWithin(bitmap.width, bitmap.height, maxEdge);
	const canvas = document.createElement('canvas');
	canvas.width = width;
	canvas.height = height;
	const ctx = canvas.getContext('2d');
	if (!ctx) {
		bitmap.close();
		throw new Error("Couldn't process that image in this browser.");
	}
	// JPEG has no transparency; a transparent PNG would otherwise come out black.
	ctx.fillStyle = '#fff';
	ctx.fillRect(0, 0, width, height);
	ctx.drawImage(bitmap, 0, 0, width, height);
	bitmap.close();

	return new Promise((resolve, reject) => {
		canvas.toBlob(
			(blob) => (blob ? resolve(blob) : reject(new Error("Couldn't process that image."))),
			'image/jpeg',
			quality
		);
	});
}
