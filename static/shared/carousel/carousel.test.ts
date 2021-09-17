/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { parse } from '../../markdown';
import { CarouselController } from './carousel';

describe('Tooltip', () => {
  let carousel: HTMLDetailsElement;
  let prevArrow: HTMLButtonElement;
  let nextArrow: HTMLButtonElement;
  let dots: HTMLElement[];
  let slides: HTMLElement[];
  let liveRegion: HTMLElement;

  beforeEach(async () => {
    document.body.innerHTML = await parse(__dirname + '/carousel.md');
    carousel = document.querySelector('.js-carousel');
    new CarouselController(carousel);
    prevArrow = carousel.querySelector('.go-Carousel-prevSlide');
    nextArrow = carousel.querySelector('.go-Carousel-nextSlide');
    dots = Array.from(carousel.querySelectorAll('.go-Carousel-dot'));
    slides = Array.from(carousel.querySelectorAll('.go-Carousel-slide'));
    liveRegion = carousel.querySelector('[aria-live]');
  });

  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('generates arrows, dots, and live region', () => {
    expect(prevArrow).toBeDefined();
    expect(nextArrow).toBeDefined();
    expect(dots).toBeDefined();
    expect(liveRegion).toBeDefined();
  });

  it('shows only the first slide', () => {
    expect(slides[0].getAttribute('aria-hidden')).toBeFalsy();
    expect(slides[1].getAttribute('aria-hidden')).toBeTruthy();
    expect(slides[2].getAttribute('aria-hidden')).toBeTruthy();
  });

  it('shows next slide on next arrow click', () => {
    nextArrow.click();
    expect(slides[0].getAttribute('aria-hidden')).toBeTruthy();
    expect(slides[1].getAttribute('aria-hidden')).toBeFalsy();
    expect(slides[2].getAttribute('aria-hidden')).toBeTruthy();
  });

  it('shows prev slide on prev arrow click', () => {
    prevArrow.click();
    expect(slides[0].getAttribute('aria-hidden')).toBeTruthy();
    expect(slides[1].getAttribute('aria-hidden')).toBeTruthy();
    expect(slides[2].getAttribute('aria-hidden')).toBeFalsy();
  });

  it('sets active slide on dot click', () => {
    dots[1].click();
    expect(slides[0].getAttribute('aria-hidden')).toBeTruthy();
    expect(slides[1].getAttribute('aria-hidden')).toBeFalsy();
    expect(slides[2].getAttribute('aria-hidden')).toBeTruthy();
  });

  it('updates live region', () => {
    expect(liveRegion.textContent).toContain('Slide 1 of 3');
    dots[1].click();
    expect(liveRegion.textContent).toContain('Slide 2 of 3');
  });
});
