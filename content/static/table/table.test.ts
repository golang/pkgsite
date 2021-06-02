/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { ExpandableRowsTableController } from './table';

describe('ExpandableRowsTableController', () => {
  let table: HTMLTableElement;
  let toggle1: HTMLButtonElement;
  let toggle2: HTMLButtonElement;
  let toggleAll: HTMLButtonElement;

  beforeEach(() => {
    document.body.innerHTML = `
      <div>
        <button class="js-toggleAll">Expand all</button>
      </div>
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
                data-aria-labelledby="toggle-id-1 label-id-2"
                data-id="toggle-id-1"
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
          <tr data-aria-controls="hidden-row-id-3">
            <td>
              <button
                type="button"
                aria-expanded="false"
                aria-label="2 more from"
                data-aria-controls="hidden-row-id-3"
                data-aria-labelledby="toggle-id-2 label-id-3"
                data-id="toggle-id-2"
              >
                +
              </button>
            </td>
            <td data-id="label-id-3">
              <span>Baz</span>
            </td>
            <td></td>
          </tr>
          <tr data-id="hidden-row-id-3">
            <td></td>
            <td>First hidden row</td>
            <td></td>
          </tr>
        </tbody>
      </table>
    `;
    table = document.querySelector<HTMLTableElement>('.js-table');
    toggleAll = document.querySelector<HTMLButtonElement>('.js-toggleAll');
    new ExpandableRowsTableController(table, toggleAll);
    toggle1 = document.querySelector<HTMLButtonElement>('#toggle-id-1');
    toggle2 = document.querySelector<HTMLButtonElement>('#toggle-id-2');
  });

  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('sets data-aria-* and data-id attributes to regular html attributes', () => {
    expect(document.querySelector('#label-id-1')).toBeTruthy();
    expect(
      document.querySelector('[aria-controls="hidden-row-id-1 hidden-row-id-2"]')
    ).toBeTruthy();
    expect(document.querySelector('[aria-labelledby="toggle-id-1 label-id-2"]')).toBeTruthy();
    expect(document.querySelector('#toggle-id-1')).toBeTruthy();
    expect(document.querySelector('#label-id-2')).toBeTruthy();
    expect(document.querySelector('#hidden-row-id-1')).toBeTruthy();
    expect(document.querySelector('#hidden-row-id-2')).toBeTruthy();
  });

  it('hides rows with unexpanded toggles', () => {
    expect(document.querySelector('#hidden-row-id-1').classList).toContain('hidden');
    expect(document.querySelector('#hidden-row-id-2').classList).toContain('hidden');
  });

  it('shows rows with expanded toggles', () => {
    toggleAll.click();
    expect(document.querySelector('#hidden-row-id-1').classList).toContain('visible');
    expect(document.querySelector('#hidden-row-id-2').classList).toContain('visible');
  });

  it('expands rows when entering text search', () => {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'f', ctrlKey: true }));
    expect(document.querySelector('#hidden-row-id-1').classList).toContain('visible');
    expect(document.querySelector('#hidden-row-id-2').classList).toContain('visible');
  });

  it('toggle expands and collapses all elements', async () => {
    jest.useFakeTimers();
    toggleAll.click();
    jest.runAllTimers();
    expect(document.querySelector('#hidden-row-id-1').classList).toContain('visible');
    expect(document.querySelector('#hidden-row-id-2').classList).toContain('visible');
    expect(toggleAll.innerText).toBe('Collapse all');
    toggleAll.click();
    jest.runAllTimers();
    expect(document.querySelector('#hidden-row-id-1').classList).toContain('hidden');
    expect(document.querySelector('#hidden-row-id-2').classList).toContain('hidden');
    expect(toggleAll.innerText).toBe('Expand all');
  });

  it('toggle changes text only when all items expanded', () => {
    jest.useFakeTimers();
    toggle1.click();
    jest.runAllTimers();
    expect(toggleAll.innerText).toBe('Expand all');
    toggle2.click();
    jest.runAllTimers();
    expect(toggleAll.innerText).toBe('Collapse all');
  });
});
