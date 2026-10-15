// One camera for the whole app. The layout owns the camera sheet, two hidden
// file inputs and a result banner; any button anywhere calls openCamera(), so a
// receipt is one tap from every page and its result shows up wherever you are.
//
// On a phone the camera is our own (CameraSheet: a live viewfinder), because the
// phone's camera — what <input capture> opens — has no way back to the camera
// roll, and a receipt is often a screenshot or a photo already taken. The sheet
// has a camera-roll button for those. Where the live camera can't run, the
// phone's own camera is the fallback; on a desktop it's a plain file picker.
//
// Opening a file input must run inside a tap's own event handler: browsers only
// let one open from a direct user gesture.

import { getAccounts, getCategories, uploadReceipt } from './api';
import { downscale } from './image';
import { describeUpload, type UploadNotice } from './receipts';

type Stage = 'preparing' | 'uploading';

export const capture = $state<{
	stage: Stage | null;
	notice: (UploadNotice & { review?: boolean }) | null;
	cameraOpen: boolean;
}>({ stage: null, notice: null, cameraOpen: false });

let cameraInput: HTMLInputElement | null = null;
let libraryInput: HTMLInputElement | null = null;
// Where the next photo goes. Most callers want the app-wide upload and banner;
// the Receipts page handles its own.
let deliver: (file: File) => void = (file) => uploadPhoto(file);
let dismissTimer: ReturnType<typeof setTimeout> | undefined;

/** The layout hands over its hidden <input type="file">s. */
export function registerCaptureInputs(camera: HTMLInputElement | null, library: HTMLInputElement | null) {
	cameraInput = camera;
	libraryInput = library;
}

/** A live camera is worth it on a touch screen that can run one. */
function liveCameraUsable(): boolean {
	return !!navigator.mediaDevices?.getUserMedia && matchMedia('(pointer: coarse)').matches;
}

/** Opens the camera; the photo, taken or picked, goes to handle. */
export function openCamera(handle: (file: File) => void = (file) => uploadPhoto(file)) {
	if (capture.stage || capture.cameraOpen) return;
	deliver = handle;
	if (liveCameraUsable()) capture.cameraOpen = true;
	else if (matchMedia('(pointer: coarse)').matches) cameraInput?.click();
	else libraryInput?.click();
}

/** The camera roll (photos, screenshots), from the camera sheet. */
export function openLibrary() {
	libraryInput?.click();
}

/** The phone's own camera, for when the live one can't start. */
export function openSystemCamera() {
	cameraInput?.click();
}

export function closeCamera() {
	capture.cameraOpen = false;
}

/** A photo arrived — from the shutter or either file input. */
export function takeFile(file: File) {
	capture.cameraOpen = false;
	deliver(file);
}

export function dismissNotice() {
	clearTimeout(dismissTimer);
	capture.notice = null;
}

/** Pages that list receipts listen for this to refresh. */
export const RECEIPT_UPLOADED = 'fangorn:receipt-uploaded';

export async function uploadPhoto(file: File) {
	dismissNotice();
	try {
		capture.stage = 'preparing';
		// Names are only needed to word the result; fetch them while the photo
		// is being shrunk and sent, and don't fail the upload over them.
		const names = Promise.all([getAccounts(), getCategories()]).catch(() => [[], []] as const);
		const image = await downscale(file);
		capture.stage = 'uploading';
		const res = await uploadReceipt(image);
		const [accounts, categories] = await names;
		const byId = (list: readonly { id: number; name: string }[]) => new Map(list.map((x) => [x.id, x.name]));
		capture.notice =
			res.receipt.status === 'needs_review' && !res.duplicate
				? {
						text: res.enabled
							? 'Read it, but it needs a look before it goes in.'
							: 'Saved. Enter the details in Receipts.',
						tone: 'warn',
						review: true
					}
				: describeUpload(res, byId(accounts), byId(categories));
		window.dispatchEvent(new CustomEvent(RECEIPT_UPLOADED));
		// Good news clears itself; anything that needs the person stays up.
		if (capture.notice.tone === 'ok') dismissTimer = setTimeout(dismissNotice, 6000);
	} catch (e) {
		capture.notice = { text: e instanceof Error ? e.message : 'Upload failed', tone: 'error' };
	} finally {
		capture.stage = null;
	}
}
