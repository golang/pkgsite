/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { parse } from '../../markdown';
import { ClipboardController } from './clipboard';

describe('ClipboardController', () => {
  const buttons: HTMLButtonElement[] = [];
  const controllers: ClipboardController[] = [];
  const dataToCopy = 'hello, world!';

  beforeAll(async () => {
    document.body.innerHTML = await parse(__dirname + '/clipboard.md');
    for (const button of document.querySelectorAll<HTMLButtonElement>('.js-clipboard')) {
      buttons.push(button);
      controllers.push(new ClipboardController(button));
    }
  });

  afterAll(() => {
    document.body.innerHTML = '';
  });

  it('copys data-to-copy when clicked', () => {
    Object.assign(navigator, { clipboard: { writeText: () => Promise.resolve() } });
    jest.spyOn(navigator.clipboard, 'writeText');
    buttons[0].click();
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(dataToCopy);
  });

  it('copys input value when clicked', () => {
    Object.assign(navigator, { clipboard: { writeText: () => Promise.resolve() } });
    jest.spyOn(navigator.clipboard, 'writeText');
    buttons[2].click();
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(dataToCopy);
  });

  it('shows error when clicked if clipboard is undefined', () => {
    Object.assign(navigator, { clipboard: undefined });
    jest.spyOn(controllers[1], 'showTooltipText');
    buttons[1].click();
    expect(controllers[1].showTooltipText).toHaveBeenCalledWith('Unable to copy', 1000);
  });
});
