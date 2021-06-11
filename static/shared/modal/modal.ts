/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * ModalController registers a dialog element with the polyfill if
 * necessary for the current browser, add adds event listeners to
 * close and open modals.
 */
export class ModalController {
  constructor(private el: HTMLDialogElement) {
    // Only load the dialog polyfill if necessary for the environment.
    if (!window.HTMLDialogElement && !el.showModal) {
      import('../../../third_party/dialog-polyfill/dialog-polyfill.esm.js').then(
        ({ default: polyfill }) => {
          polyfill.registerDialog(el);
        }
      );
    }
    const id = el.id;
    const button = document.querySelector<HTMLButtonElement>(`[aria-controls="${id}"]`);
    if (button) {
      button.addEventListener('click', () => {
        if (this.el.showModal) {
          this.el.showModal();
        } else {
          this.el.open = true;
        }
        el.querySelector('input')?.focus();
      });
    }
    for (const close of this.el.querySelectorAll<HTMLButtonElement>('[data-modal-close]')) {
      close.addEventListener('click', () => {
        if (this.el.close) {
          this.el.close();
        } else {
          this.el.open = false;
        }
      });
    }
  }
}
