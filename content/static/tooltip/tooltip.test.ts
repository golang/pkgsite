/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { parse } from '../../markdown';
import { ToolTipController } from './tooltip';

describe('Tooltip', () => {
  let tooltip: HTMLDetailsElement;
  let summary: HTMLElement;

  beforeEach(async () => {
    document.body.innerHTML = await parse(__dirname + '/tooltip.md');
    tooltip = document.querySelector('.js-tooltip');
    summary = tooltip.firstElementChild as HTMLElement;
    new ToolTipController(tooltip);
    summary.click();
  });

  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('opens', () => {
    expect(tooltip.open).toBeTruthy();
  });

  it('closes on click', () => {
    summary.click();
    expect(tooltip.open).toBeFalsy();
  });

  it('closes on outside click', () => {
    document.body.click();
    expect(tooltip.open).toBeFalsy();
  });
});
