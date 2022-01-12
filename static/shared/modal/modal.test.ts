/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { parse } from '../../markdown';
import { ModalController } from './modal';

describe('Modal', () => {
  let modal: HTMLDialogElement;
  let openButton: HTMLButtonElement;
  let closeButton: HTMLButtonElement;

  beforeEach(async () => {
    document.body.innerHTML = await parse(__dirname + '/modal.md');
    modal = document.querySelector('#example-modal-id1');
    openButton = document.querySelector('[aria-controls="example-modal-id1"]');
    new ModalController(modal);
    openButton.click();
    closeButton = document.querySelector('[data-modal-close]');
  });

  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('opens', () => {
    expect(modal.getAttribute('opened')).toBeTruthy();
  });

  it('closes on cancel', async () => {
    closeButton.click();
    expect(modal.getAttribute('opened')).toBeFalsy();
  });
});
