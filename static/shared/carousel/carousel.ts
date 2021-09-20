/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * Carousel Controller adds event listeners, accessibilty enhancements, and
 * control elements to a carousel component.
 */
export class CarouselController {
  /**
   * slides is a collection of slides in the carousel.
   */
  private slides: HTMLLIElement[];
  /**
   * dots is a collection of dot navigation controls, added to the carousel
   * by this controller.
   */
  private dots: HTMLElement[];
  /**
   * liveRegion is a visually hidden element that notifies assitive devices
   * of visual changes to the carousel. They are added to the carousel by
   * this controller.
   */
  private liveRegion: HTMLElement;
  /**
   * activeIndex is the 0-index of the currently active slide.
   */
  private activeIndex: number;

  constructor(private el: HTMLElement) {
    this.slides = Array.from(el.querySelectorAll('.go-Carousel-slide'));
    this.dots = [];
    this.liveRegion = document.createElement('div');
    this.activeIndex = Number(el.getAttribute('data-slide-index') ?? 0);

    this.initSlides();
    this.initArrows();
    this.initDots();
    this.initLiveRegion();
  }

  private initSlides() {
    for (const [i, v] of this.slides.entries()) {
      if (i === this.activeIndex) continue;
      v.setAttribute('aria-hidden', 'true');
    }
  }

  private initArrows() {
    const arrows = document.createElement('ul');
    arrows.classList.add('go-Carousel-arrows');
    arrows.innerHTML = `
      <li>
        <button class="go-Carousel-prevSlide" aria-label="Go to previous slide">
          <img class="go-Icon" height="24" width="24" src="/static/shared/icon/arrow_left_gm_grey_24dp.svg" alt="">
        </button>
      </li>
      <li>
        <button class="go-Carousel-nextSlide" aria-label="Go to next slide">
          <img class="go-Icon" height="24" width="24" src="/static/shared/icon/arrow_right_gm_grey_24dp.svg" alt="">
        </button>
      </li>
    `;
    arrows
      .querySelector('.go-Carousel-prevSlide')
      ?.addEventListener('click', () => this.setActive(this.activeIndex - 1));
    arrows
      .querySelector('.go-Carousel-nextSlide')
      ?.addEventListener('click', () => this.setActive(this.activeIndex + 1));
    this.el.append(arrows);
  }

  private initDots() {
    const dots = document.createElement('ul');
    dots.classList.add('go-Carousel-dots');
    for (let i = 0; i < this.slides.length; i++) {
      const li = document.createElement('li');
      const button = document.createElement('button');
      button.classList.add('go-Carousel-dot');
      if (i === this.activeIndex) {
        button.classList.add('go-Carousel-dot--active');
      }
      button.innerHTML = `<span class="go-Carousel-obscured">Slide ${i + 1}</span>`;
      button.addEventListener('click', () => this.setActive(i));
      li.append(button);
      dots.append(li);
      this.dots.push(button);
    }
    this.el.append(dots);
  }

  private initLiveRegion() {
    this.liveRegion.setAttribute('aria-live', 'polite');
    this.liveRegion.setAttribute('aria-atomic', 'true');
    this.liveRegion.setAttribute('class', 'go-Carousel-obscured');
    this.liveRegion.textContent = `Slide ${this.activeIndex + 1} of ${this.slides.length}`;
    this.el.appendChild(this.liveRegion);
  }

  private setActive = (index: number) => {
    this.activeIndex = (index + this.slides.length) % this.slides.length;
    this.el.setAttribute('data-slide-index', String(this.activeIndex));
    for (const d of this.dots) {
      d.classList.remove('go-Carousel-dot--active');
    }
    this.dots[this.activeIndex].classList.add('go-Carousel-dot--active');
    for (const s of this.slides) {
      s.setAttribute('aria-hidden', 'true');
    }
    this.slides[this.activeIndex].removeAttribute('aria-hidden');
    this.liveRegion.textContent = 'Slide ' + (this.activeIndex + 1) + ' of ' + this.slides.length;
  };
}
