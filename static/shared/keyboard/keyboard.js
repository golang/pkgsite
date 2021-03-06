/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */import{track as a}from"../analytics/analytics";class s{constructor(){this.handlers={},document.addEventListener("keydown",e=>this.handleKeyPress(e))}on(e,t,r,n){return this.handlers[e]??=new Set,this.handlers[e].add({description:t,callback:r,...n}),this}handleKeyPress(e){for(const t of this.handlers[e.key]??new Set){if(t.target&&t.target!==e.target)return;const r=e.target;if(!t.target&&(r?.tagName==="INPUT"||r?.tagName==="SELECT"||r?.tagName==="TEXTAREA")||r?.isContentEditable||t.withMeta&&!(e.ctrlKey||e.metaKey)||!t.withMeta&&(e.ctrlKey||e.metaKey))return;a("keypress","hotkeys",`${e.key} pressed`,t.description),t.callback(e)}}}export const keyboard=new s;
//# sourceMappingURL=keyboard.js.map
