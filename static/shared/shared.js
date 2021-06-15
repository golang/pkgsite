/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */import"./header/header";import{ClipboardController as e}from"./clipboard/clipboard";import{ToolTipController as l}from"./tooltip/tooltip";import{SelectNavController as t}from"./outline/select";import{ModalController as r}from"./modal/modal";export{keyboard}from"./keyboard/keyboard";for(const o of document.querySelectorAll(".js-clipboard"))new e(o);for(const o of document.querySelectorAll(".js-modal"))new r(o);for(const o of document.querySelectorAll(".js-tooltip"))new l(o);for(const o of document.querySelectorAll(".js-selectNav"))new t(o);
//# sourceMappingURL=shared.js.map
