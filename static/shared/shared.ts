/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import './header/header';
import './keyboard/keyboard';
import { ClipboardController } from './clipboard/clipboard';
import { ToolTipController } from './tooltip/tooltip';
import { SelectNavController } from './outline/select';
import { ModalController } from './modal/modal';

for (const el of document.querySelectorAll<HTMLButtonElement>('.js-clipboard')) {
  new ClipboardController(el);
}

for (const el of document.querySelectorAll<HTMLDialogElement>('.js-modal')) {
  new ModalController(el);
}

for (const t of document.querySelectorAll<HTMLDetailsElement>('.js-tooltip')) {
  new ToolTipController(t);
}

for (const el of document.querySelectorAll<HTMLSelectElement>('.js-selectNav')) {
  new SelectNavController(el);
}
