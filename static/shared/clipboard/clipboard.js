/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */export class ClipboardController{constructor(t){this.el=t;this.data=t.dataset.toCopy??t.innerText,!this.data&&t.parentElement?.classList.contains("go-InputGroup")&&(this.data=(this.data||t.parentElement?.querySelector("input")?.value)??""),t.addEventListener("click",e=>this.handleCopyClick(e))}handleCopyClick(t){t.preventDefault();const e=1e3;if(!navigator.clipboard){this.showTooltipText("Unable to copy",e);return}navigator.clipboard.writeText(this.data).then(()=>{this.showTooltipText("Copied!",e)}).catch(()=>{this.showTooltipText("Unable to copy",e)})}showTooltipText(t,e){this.el.setAttribute("data-tooltip",t),setTimeout(()=>this.el.setAttribute("data-tooltip",""),e)}}
//# sourceMappingURL=clipboard.js.map
