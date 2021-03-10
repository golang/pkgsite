/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */export class CopyToClipboardController{constructor(t){this._el=t,this._data=t.dataset.toCopy??"",t.addEventListener("click",o=>this.handleCopyClick(o))}handleCopyClick(t){t.preventDefault();const o=1e3;if(!navigator.clipboard){this.showTooltipText("Unable to copy",o);return}navigator.clipboard.writeText(this._data).then(()=>{this.showTooltipText("Copied!",o)}).catch(()=>{this.showTooltipText("Unable to copy",o)})}showTooltipText(t,o){this._el.setAttribute("data-tooltip",t),setTimeout(()=>this._el.setAttribute("data-tooltip",""),o)}}
//# sourceMappingURL=clipboard.js.map
