/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { track } from '../analytics/analytics';

/**
 * Options are keyhandler callback options.
 */
interface Options {
  /**
   * target is the element the key event should filter on. The
   * default target is the document.
   */
  target?: Element;

  /**
   * withMeta specifies if the event callback should fire when
   * the key is pressed with a meta key (ctrl, alt, etc). By
   * default meta keypresses are ignored.
   */
  withMeta?: boolean;
}

/**
 * KeyHandler is the config for a keyboard event callback.
 */
interface KeyHandler extends Options {
  description: string;
  callback: (e: KeyboardEvent) => void;
}

/**
 * KeyboardController controls event callbacks for sitewide
 * keyboard events. Multiple callbacks can be registered for
 * a single key and by default the controller ignores events
 * for text input targets.
 */
class KeyboardController {
  handlers: Record<string, Set<KeyHandler>>;

  constructor() {
    this.handlers = {};
    document.addEventListener('keydown', e => this.handleKeyPress(e));
  }

  /**
   * on registers keyboard event callbacks.
   * @param key the key to register.
   * @param description name of the event.
   * @param callback event callback.
   * @param options set target and withMeta options to override the default behaviors.
   */
  on(key: string, description: string, callback: (e: KeyboardEvent) => void, options?: Options) {
    this.handlers[key] ??= new Set();
    this.handlers[key].add({ description, callback, ...options });
    return this;
  }

  private handleKeyPress(e: KeyboardEvent) {
    for (const handler of this.handlers[e.key] ?? new Set()) {
      if (handler.target && handler.target !== e.target) {
        return;
      }
      const t = e.target as HTMLElement | null;
      if (
        !handler.target &&
        (t?.tagName === 'INPUT' || t?.tagName === 'SELECT' || t?.tagName === 'TEXTAREA')
      ) {
        return;
      }
      if (t?.isContentEditable) {
        return;
      }
      if (
        (handler.withMeta && !(e.ctrlKey || e.metaKey)) ||
        (!handler.withMeta && (e.ctrlKey || e.metaKey))
      ) {
        return;
      }
      track('keypress', 'hotkeys', `${e.key} pressed`, handler.description);
      handler.callback(e);
    }
  }
}

export const keyboard = new KeyboardController();
