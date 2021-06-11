/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { CopyToClipboardController } from './clipboard';

describe('CopyToClipboardController', () => {
  let button: HTMLButtonElement;
  let controller: CopyToClipboardController;
  const dataToCopy = 'Hello, world!';

  beforeEach(() => {
    document.body.innerHTML = `
      <div>
        <button class="js-copyToClipboard" data-to-copy="${dataToCopy}"></button>
      </div>
    `;
    button = document.querySelector<HTMLButtonElement>('.js-copyToClipboard');
    controller = new CopyToClipboardController(button);
  });

  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('copys text when clicked', () => {
    Object.assign(navigator, { clipboard: { writeText: () => Promise.resolve() } });
    jest.spyOn(navigator.clipboard, 'writeText');
    button.click();
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(dataToCopy);
  });

  it('shows error when clicked if clipboard is undefined', () => {
    Object.assign(navigator, { clipboard: undefined });
    jest.spyOn(controller, 'showTooltipText');
    button.click();
    expect(controller.showTooltipText).toHaveBeenCalledWith('Unable to copy', 1000);
  });
});
