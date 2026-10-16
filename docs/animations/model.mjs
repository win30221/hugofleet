// A deterministic playback clock. It never contacts Hub, Kubernetes or a shell.
export const STEP_SECONDS = 10;
export const stepSeconds = (step) => step.seconds || STEP_SECONDS;
export const chapterSeconds = (chapter) => chapter.steps.reduce((sum, step) => sum + stepSeconds(step), 0);
export const stepStart = (chapter, index) => chapter.steps.slice(0, index).reduce((sum, step) => sum + stepSeconds(step), 0);
export const clamp = (n, lo, hi) => Math.min(hi, Math.max(lo, n));
export const ease = (n) => { n = clamp(n, 0, 1); return n * n * (3 - 2 * n); };
export function sample(chapter, seconds) {
  const time = clamp(seconds, 0, chapterSeconds(chapter));
  let index = 0;
  while (index < chapter.steps.length - 1 && time >= stepStart(chapter, index + 1)) index++;
  const within = clamp((time - stepStart(chapter, index)) / stepSeconds(chapter.steps[index]), 0, 1);
  const step = chapter.steps[index];
  const sequence = step.sequence || [];
  let eventIndex = 0;
  while (eventIndex < sequence.length - 1 && within + 1e-8 >= sequence[eventIndex + 1].at) eventIndex++;
  const event = sequence[eventIndex] || {};
  const state = { pods: 1, bpods: 0, desired: 1, revision: 1, connected: true, gate: true, ...step.state, ...event };
  // The operation shuttle has a short approach, a delivery, and a long reading hold.
  const delivery = ease(within / 0.38);
  const scan = state.scan ? Math.sin(within * Math.PI * 4) * 15 : 0;
  return { index, within, step, state, delivery, scan, time, eventIndex, event };
}
