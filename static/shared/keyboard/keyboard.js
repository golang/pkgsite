/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */import{track as i}from"../analytics/analytics";class o{constructor(){this.handlers={},document.addEventListener("keydown",e=>this.handleKeyPress(e))}on(e,a,r,t){var n,s;return(s=(n=this.handlers)[e])!=null||(n[e]=new Set),this.handlers[e].add({description:a,callback:r,...t}),this}handleKeyPress(e){var a;for(const r of(a=this.handlers[e.key.toLowerCase()])!=null?a:new Set){if(r.target&&r.target!==e.target)return;const t=e.target;if(!r.target&&((t==null?void 0:t.tagName)==="INPUT"||(t==null?void 0:t.tagName)==="SELECT"||(t==null?void 0:t.tagName)==="TEXTAREA")||(t==null?void 0:t.isContentEditable)||r.withMeta&&!(e.ctrlKey||e.metaKey)||!r.withMeta&&(e.ctrlKey||e.metaKey))return;i("keypress","hotkeys",`${e.key} pressed`,r.description),r.callback(e)}}}const c=new o;export{c as keyboard};
//# sourceMappingURL=keyboard.js.map
