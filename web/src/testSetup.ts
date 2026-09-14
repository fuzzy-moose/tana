// jsdom has no layout engine or pointer capture; Radix observes and scrolls controls.
if (typeof window !== 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  HTMLElement.prototype.scrollIntoView = () => {}
  HTMLElement.prototype.hasPointerCapture = () => false
  HTMLElement.prototype.setPointerCapture = () => {}
  HTMLElement.prototype.releasePointerCapture = () => {}
}
