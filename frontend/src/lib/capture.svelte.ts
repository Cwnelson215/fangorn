// One camera for the whole app. The layout owns a single hidden file input and
// a result banner; any button anywhere calls openCamera(), so a receipt is one
// tap from every page and its result shows up wherever you are.
//
// openCamera() must run inside the tap's own event handler: browsers only let a
// file input open from a direct user gesture.

import { getAccounts, getCategories, uploadReceipt } from './api';
import { downscale } from './image';
import { describeUpload, type UploadNotice } from './receipts';

type Stage = 'preparing' | 'uploading';

export const capture = $state<{
	stage: Stage | null;
	notice: (UploadNotice & { review?: boolean }) | null;
}>({ stage: null, notice: null });

let input: HTMLInputElement | null = null;
let dismissTimer: ReturnType<typeof setTimeout> | undefined;

/** The layout hands over its hidden <input type="file">. */
export function registerCameraInput(el: HTMLInputElement | null) {
	input = el;
}

export function openCamera() {
	if (capture.stage) return;
	input?.click();
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
