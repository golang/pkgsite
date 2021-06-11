/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { parse } from '../../markdown';
import { TreeNavController } from './tree';
import { makeSelectNav } from './select';

const observe = jest.fn();
window.IntersectionObserver = jest.fn(() => ({
  observe,
})) as any;

let treeEl: HTMLElement;
let tree: TreeNavController;
let selectNav: HTMLElement;

beforeEach(async () => {
  document.body.innerHTML = await parse(__dirname + '/outline.md');
  treeEl = document.querySelector('.js-tree') as HTMLElement;
  tree = new TreeNavController(treeEl);
  selectNav = makeSelectNav(tree);
});

afterEach(() => {
  document.body.innerHTML = '';
});

it('creates select nav from tree', () => {
  expect(selectNav).toMatchSnapshot();
});
