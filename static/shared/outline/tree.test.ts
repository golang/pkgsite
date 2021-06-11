/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { parse } from '../../markdown';
import { TreeNavController } from './tree';

const observe = jest.fn();
window.IntersectionObserver = jest.fn(() => ({
  observe,
})) as any;

let treeEl: HTMLElement;
let a1: HTMLElement;
let a21: HTMLElement;
let a22: HTMLElement;
let a23: HTMLElement;
let a31: HTMLElement;
let a41: HTMLElement;
let a42: HTMLElement;

beforeEach(async () => {
  document.body.innerHTML = await parse(__dirname + '/outline.md');
  treeEl = document.querySelector('.js-tree') as HTMLElement;
  new TreeNavController(treeEl);
  a1 = a('#one');
  a21 = a('#two-one');
  a22 = a('#two-two');
  a23 = a('#two-three');
  a31 = a('#three-one');
  a41 = a('#four-one');
  a42 = a('#four-two');
  a1.focus();
});

afterEach(() => {
  document.body.innerHTML = '';
});

it('creates tree nav from ul', () => {
  expect(treeEl).toMatchSnapshot();
});

it('adds role=group attribute to ul elements', () => {
  for (const ul of treeEl.querySelectorAll('ul')) {
    expect(attr(ul, 'role')).toBe('group');
  }
});

it('adds role=none attribute to li elements', () => {
  for (const ul of treeEl.querySelectorAll('li')) {
    expect(attr(ul, 'role')).toBe('none');
  }
});

it('adds role=treeitem attribute to a elements', () => {
  for (const ul of treeEl.querySelectorAll('a')) {
    expect(attr(ul, 'role')).toBe('treeitem');
  }
});

it('adds aria-expanded role to tree items with children', () => {
  expect(attr(a1, 'aria-expanded')).toBe('false');
  expect(attr(a22, 'aria-expanded')).toBe('false');
  expect(attr(a31, 'aria-expanded')).toBe('false');
});

it('does not add aria-expanded role to tree items with children', () => {
  expect(attr(a21, 'aria-expanded')).toBeNull();
  expect(attr(a41, 'aria-expanded')).toBeNull();
  expect(attr(a42, 'aria-expanded')).toBeNull();
});

it('adds aria-level to tree items', () => {
  expect(attr(a1, 'aria-level')).toBe('1');
  expect(attr(a21, 'aria-level')).toBe('2');
  expect(attr(a22, 'aria-level')).toBe('2');
  expect(attr(a31, 'aria-level')).toBe('3');
  expect(attr(a41, 'aria-level')).toBe('4');
  expect(attr(a42, 'aria-level')).toBe('4');
});

it('focuses tree item on click', () => {
  a1.click();
  expect(attr(a1, 'aria-expanded')).toBe('true');
  expect(attr(a1, 'aria-selected')).toBe('true');
});

it('closes unfocused branches', () => {
  a1.click();
  expect(attr(a1, 'aria-expanded')).toBe('true');
  expect(attr(a1, 'aria-selected')).toBe('true');
  a22.click();
  expect(attr(a22, 'aria-expanded')).toBe('true');
  expect(attr(a22, 'aria-selected')).toBe('true');
  a21.click();
  expect(attr(a22, 'aria-expanded')).toBe('false');
  expect(attr(a22, 'aria-selected')).toBe('false');
});

it('navigates treeitems with the keyboard', () => {
  keydown(a1, 'ArrowRight'); // expand a1
  expect(attr(a1, 'aria-expanded')).toBe('true');

  keydown(a1, 'ArrowRight'); // focus a21
  expect(attr(a1, 'tabindex')).toBe('-1');
  expect(attr(a21, 'tabindex')).toBe('0');

  keydown(a21, 'ArrowDown'); // focus a22
  expect(attr(a21, 'tabindex')).toBe('-1');
  expect(attr(a22, 'tabindex')).toBe('0');

  keydown(a22, 'ArrowRight'); // expand a22
  expect(attr(a22, 'aria-expanded')).toBe('true');

  keydown(a22, 'ArrowRight'); // focus a31
  expect(attr(a22, 'tabindex')).toBe('-1');
  expect(attr(a31, 'tabindex')).toBe('0');

  keydown(a31, 'ArrowUp'); // focus a22
  expect(attr(a31, 'tabindex')).toBe('-1');
  expect(attr(a22, 'tabindex')).toBe('0');

  keydown(a22, 'ArrowUp'); // focus a21
  expect(attr(a22, 'tabindex')).toBe('-1');
  expect(attr(a21, 'tabindex')).toBe('0');

  keydown(a21, 'ArrowLeft'); // focus a1
  expect(attr(a21, 'tabindex')).toBe('-1');
  expect(attr(a1, 'tabindex')).toBe('0');

  keydown(a1, 'End'); // focus a31
  expect(attr(a1, 'tabindex')).toBe('-1');
  expect(attr(a23, 'tabindex')).toBe('0');

  keydown(a23, 'Home'); // focus a1
  expect(attr(a31, 'tabindex')).toBe('-1');
  expect(attr(a1, 'tabindex')).toBe('0');
});

it('expands sibling items with * key', () => {
  keydown(a1, 'ArrowRight');
  keydown(a1, 'ArrowDown');
  keydown(a21, '*');
  expect(attr(a22, 'aria-expanded')).toBe('true');
  expect(attr(a23, 'aria-expanded')).toBe('true');
});

function a(href: string): HTMLElement {
  return document.querySelector(`[href="${href}"]`);
}

function attr(el: Element, name: string): string {
  return el.getAttribute(name);
}

function keydown(el: Element, key: string): void {
  el.dispatchEvent(new KeyboardEvent('keydown', { key }));
}
