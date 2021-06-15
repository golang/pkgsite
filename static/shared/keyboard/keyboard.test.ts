/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { keyboard } from './keyboard';

describe('KeyboardController', () => {
  const fn = jest.fn();
  const create = (el: string) => document.body.appendChild(document.createElement(el));
  const target = create('div');
  const input = create('input');
  const select = create('select');
  const textarea = create('textarea');
  const editableDiv = create('div');
  editableDiv.contentEditable = 'true';
  const bubbleDiv = create('div');
  const nonTargetDiv = create('div');

  keyboard
    .on('y', 'default event', () => fn())
    .on('m', 'meta required', () => fn(), { withMeta: true })
    .on('t', 'target required', () => fn(), { target: target });

  beforeEach(() => {
    jest.resetAllMocks();
  });

  it('fires a callback when a key is down', () => {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'y' }));
    expect(fn).toBeCalled();
  });

  it('skips callback when a meta key is used', () => {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'y', metaKey: true }));
    expect(fn).not.toBeCalled();
  });

  it('fires callback when a meta key is required', () => {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'm', metaKey: true }));
    expect(fn).toBeCalled();
  });

  it('skips callback when a meta key is missing', () => {
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'm' }));
    expect(fn).not.toBeCalled();
  });

  it('skips callback when target is an input element', () => {
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'y' }));
    expect(fn).not.toBeCalled();
  });

  it('skips callback when target is a select element', () => {
    select.dispatchEvent(new KeyboardEvent('keydown', { key: 'y' }));
    expect(fn).not.toBeCalled();
  });

  it('skips callback when target is a textarea element', () => {
    textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'y' }));
    expect(fn).not.toBeCalled();
  });

  it('skips callback when target is a an editable element', () => {
    editableDiv.dispatchEvent(new KeyboardEvent('keydown', { key: 'y' }));
    expect(fn).not.toBeCalled();
  });

  it('fires callback when event bubbles from target', () => {
    bubbleDiv.dispatchEvent(new KeyboardEvent('keydown', { key: 'y', bubbles: true }));
    expect(fn).toBeCalled();
  });

  it('fires callback when event matches target', () => {
    target.dispatchEvent(new KeyboardEvent('keydown', { key: 't', bubbles: true }));
    expect(fn).toBeCalled();
  });

  it('skips callback when event does not match target', () => {
    nonTargetDiv.dispatchEvent(new KeyboardEvent('keydown', { key: 't', bubbles: true }));
    expect(fn).not.toBeCalled();
  });
});
