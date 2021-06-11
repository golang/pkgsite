/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
export class ToolTipController {
  constructor(el) {
    this.el = el;
    document.addEventListener("click", (e) => {
      const insideTooltip = this.el.contains(e.target);
      if (!insideTooltip) {
        this.el.removeAttribute("open");
      }
    });
  }
}
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsidG9vbHRpcC50cyJdLAogICJzb3VyY2VzQ29udGVudCI6IFsiLyoqXG4gKiBAbGljZW5zZVxuICogQ29weXJpZ2h0IDIwMjEgVGhlIEdvIEF1dGhvcnMuIEFsbCByaWdodHMgcmVzZXJ2ZWQuXG4gKiBVc2Ugb2YgdGhpcyBzb3VyY2UgY29kZSBpcyBnb3Zlcm5lZCBieSBhIEJTRC1zdHlsZVxuICogbGljZW5zZSB0aGF0IGNhbiBiZSBmb3VuZCBpbiB0aGUgTElDRU5TRSBmaWxlLlxuICovXG5cbi8qKlxuICogVG9vbFRpcENvbnRyb2xsZXIgaGFuZGxlcyBjbG9zaW5nIHRvb2x0aXBzIG9uIGV4dGVybmFsIGNsaWNrcy5cbiAqL1xuZXhwb3J0IGNsYXNzIFRvb2xUaXBDb250cm9sbGVyIHtcbiAgY29uc3RydWN0b3IocHJpdmF0ZSBlbDogSFRNTERldGFpbHNFbGVtZW50KSB7XG4gICAgZG9jdW1lbnQuYWRkRXZlbnRMaXN0ZW5lcignY2xpY2snLCBlID0+IHtcbiAgICAgIGNvbnN0IGluc2lkZVRvb2x0aXAgPSB0aGlzLmVsLmNvbnRhaW5zKGUudGFyZ2V0IGFzIEVsZW1lbnQpO1xuICAgICAgaWYgKCFpbnNpZGVUb29sdGlwKSB7XG4gICAgICAgIHRoaXMuZWwucmVtb3ZlQXR0cmlidXRlKCdvcGVuJyk7XG4gICAgICB9XG4gICAgfSk7XG4gIH1cbn1cbiJdLAogICJtYXBwaW5ncyI6ICJBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUE7QUFBQTtBQVVPLCtCQUF3QjtBQUFBLEVBQzdCLFlBQW9CLElBQXdCO0FBQXhCO0FBQ2xCLGFBQVMsaUJBQWlCLFNBQVMsT0FBSztBQUN0QyxZQUFNLGdCQUFnQixLQUFLLEdBQUcsU0FBUyxFQUFFO0FBQ3pDLFVBQUksQ0FBQyxlQUFlO0FBQ2xCLGFBQUssR0FBRyxnQkFBZ0I7QUFBQTtBQUFBO0FBQUE7QUFBQTsiLAogICJuYW1lcyI6IFtdCn0K
