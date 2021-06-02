/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */import"../header/header.js";import"../keyboard/keyboard.js";import{ClipboardController as n}from"../clipboard/clipboard.js";import{ToolTipController as a}from"../tooltip/tooltip.js";import{SelectNavController as l}from"../outline/select.js";import{ModalController as i}from"../modal/modal.js";for(const e of document.querySelectorAll(".js-clipboard"))new n(e);for(const e of document.querySelectorAll(".js-modal"))new i(e);for(const e of document.querySelectorAll(".js-tooltip"))new a(e);for(const e of document.querySelectorAll(".js-selectNav"))new l(e);(function(){window.dataLayer=window.dataLayer||[],window.dataLayer.push({"gtm.start":new Date().getTime(),event:"gtm.js"})})();function r(){const e=new URLSearchParams(window.location.search),o=e.get("utm_source");if(o!=="gopls"&&o!=="godoc"&&o!=="pkggodev")return;const t=new URL(window.location.href);e.delete("utm_source"),t.search=e.toString(),window.history.replaceState(null,"",t.toString())}document.querySelector(".js-gtmID")?.dataset.gtmid&&window.dataLayer?window.dataLayer.push(function(){r()}):r();
//# sourceMappingURL=base.js.map
