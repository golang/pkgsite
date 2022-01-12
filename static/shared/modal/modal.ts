/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

interface Window {
  dialogPolyfill?: {
    registerDialog: (el: HTMLDialogElement) => void;
  };
}

declare const window: Window;

/**
 * ModalController registers a dialog element with the polyfill if
 * necessary for the current browser, add adds event listeners to
 * close and open modals.
 */
export class ModalController {
  constructor(private el: HTMLDialogElement) {
    if (window.dialogPolyfill) {
      window.dialogPolyfill.registerDialog(el);
    }
    this.init();
  }

  init() {
    const button = document.querySelector<HTMLButtonElement>(`[aria-controls="${this.el.id}"]`);
    if (button) {
      button.addEventListener('click', () => {
        if (this.el.showModal) {
          this.el.showModal();
        } else {
          this.el.setAttribute('opened', 'true');
        }
        this.el.querySelector('input')?.focus();
      });
    }
    for (const btn of this.el.querySelectorAll<HTMLButtonElement>('[data-modal-close]')) {
      btn.addEventListener('click', () => {
        if (this.el.close) {
          this.el.close();
        } else {
          this.el.removeAttribute('opened');
        }
      });
    }
  }
}
