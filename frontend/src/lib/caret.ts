// Textareas do not expose a caret rectangle. Mirror their typography and wrapping
// in an invisible element, then measure a marker at the selection boundary.
export function caretRect(input: HTMLTextAreaElement, position = input.selectionStart) {
  const mirror = document.createElement('div');
  const style = getComputedStyle(input);
  for (const property of [
    'box-sizing',
    'font-family',
    'font-size',
    'font-weight',
    'font-style',
    'line-height',
    'letter-spacing',
    'word-spacing',
    'text-indent',
    'text-transform',
    'tab-size',
    'padding-top',
    'padding-right',
    'padding-bottom',
    'padding-left',
    'border-top-width',
    'border-right-width',
    'border-bottom-width',
    'border-left-width',
  ]) {
    mirror.style.setProperty(property, style.getPropertyValue(property));
  }
  Object.assign(mirror.style, {
    position: 'fixed',
    visibility: 'hidden',
    left: '0',
    top: '0',
    width: `${input.clientWidth}px`,
    boxSizing: 'border-box',
    border: '0',
    whiteSpace: 'pre-wrap',
    overflowWrap: 'break-word',
  });
  mirror.textContent = input.value.slice(0, position);
  const marker = document.createElement('span');
  marker.textContent = input.value.slice(position) || '\u200b';
  mirror.append(marker);
  document.body.append(mirror);
  const point = marker.getClientRects()[0];
  const bounds = input.getBoundingClientRect();
  const lineHeight = parseFloat(style.lineHeight) || parseFloat(style.fontSize) * 1.5;
  const result = {
    left: bounds.left + point.left - input.scrollLeft,
    top: bounds.top + point.top - input.scrollTop,
    bottom: bounds.top + point.top - input.scrollTop + lineHeight,
  };
  mirror.remove();
  return result;
}
