/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { ExpandableRowsTableController } from './table';

describe('ExpandableRowsTableController', () => {
  let table: HTMLTableElement;
  let toggle: HTMLButtonElement;

  beforeEach(() => {
    document.body.innerHTML = `
      <table class="js-table">
        <tbody>
          <tr>
            <th>Toggle</th>
            <th>Foo</th>
            <th>Bar</th>
          </tr>
          <tr>
            <td></td>
            <td data-id="label-id-1">Hello World</td>
            <td>Simple row with no toggle or hidden elements</td>
          </tr>
          <tr data-aria-controls="hidden-row-id-1 hidden-row-id-2">
            <td>
              <button
                type="button"
                aria-expanded="false"
                aria-label="2 more from"
                data-aria-controls="hidden-row-id-1 hidden-row-id-2"
                data-aria-labelledby="toggle-id label-id-2"
                data-id="toggle-id"
              >
                +
              </button>
            </td>
            <td data-id="label-id-2">
              <span>Baz</span>
            </td>
            <td></td>
          </tr>
          <tr data-id="hidden-row-id-1">
            <td></td>
            <td>First hidden row</td>
            <td></td>
          </tr>
          <tr data-id="hidden-row-id-2">
            <td></td>
            <td>Second hidden row</td>
            <td></td>
          </tr>
        </tbody>
      </table>
    `;
    table = document.querySelector<HTMLTableElement>('.js-table');
    new ExpandableRowsTableController(table);
    toggle = document.querySelector<HTMLButtonElement>('#toggle-id');
  });

  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('sets data-aria-* and data-id attributes to regular html attributes', () => {
    expect(document.querySelector('#label-id-1')).toBeTruthy();
    expect(
      document.querySelector('[aria-controls="hidden-row-id-1 hidden-row-id-2"]')
    ).toBeTruthy();
    expect(document.querySelector('[aria-labelledby="toggle-id label-id-2"]')).toBeTruthy();
    expect(document.querySelector('#toggle-id')).toBeTruthy();
    expect(document.querySelector('#label-id-2')).toBeTruthy();
    expect(document.querySelector('#hidden-row-id-1')).toBeTruthy();
    expect(document.querySelector('#hidden-row-id-2')).toBeTruthy();
  });

  it('hides rows with unexpanded toggles', () => {
    expect(document.querySelector('#hidden-row-id-1').classList).toContain('hidden');
    expect(document.querySelector('#hidden-row-id-2').classList).toContain('hidden');
  });

  it('shows rows with expanded toggles', () => {
    toggle.click();
    expect(document.querySelector('#hidden-row-id-1').classList).toContain('visible');
    expect(document.querySelector('#hidden-row-id-2').classList).toContain('visible');
  });
});
